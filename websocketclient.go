package main

import (
	"encoding/json"
	"errors"
	"log"
	"math"
	"sync"

	ucapi "github.com/enbility/eebus-go/usecases/api"
	shipapi "github.com/enbility/ship-go/api"
	"github.com/gorilla/websocket"
)

type WebsocketClient struct {
	websocket *websocket.Conn
	mutex     sync.Mutex
	mutex2    sync.Mutex
}

func (websocketClient *WebsocketClient) sendMessage(msg interface{}) error {
	if websocketClient.websocket == nil {
		return errors.New("no frontend connected")
	}

	// Encode first: an unencodable message must not drop the connection.
	data, err := json.Marshal(msg)
	if err != nil {
		log.Println("websocket encode error:", err)
		return err
	}

	websocketClient.mutex.Lock()
	defer websocketClient.mutex.Unlock()

	err = websocketClient.websocket.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		if !websocket.IsCloseError(err,
			websocket.CloseNormalClosure,
			websocket.CloseGoingAway,
			websocket.CloseNoStatusReceived,
		) {
			log.Println("websocket write error:", err)
		}
		websocketClient.websocket = nil
	}

	return err
}

// finite replaces values JSON cannot encode. Devices like the Weidmueller ERC
// report "no limit" as +Inf, which is signaled to the frontend via
// Message.Unlimited instead.
func finite(value float64) float64 {
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return 0
	}
	return value
}

func (websocketClient *WebsocketClient) sendNotification(ski string, messageType int, uc string) error {
	answer := Message{
		SKI:     ski,
		Type:    messageType,
		UseCase: uc}

	return websocketClient.sendMessage(answer)
}

func (websocketClient *WebsocketClient) sendText(messageType int, text string) error {
	answer := Message{
		Type: messageType,
		Text: text}

	return websocketClient.sendMessage(answer)
}

func (websocketClient *WebsocketClient) sendValue(ski string, messageType int, useCase string, value float64) error {
	answer := Message{
		SKI:       ski,
		Type:      messageType,
		Value:     finite(value),
		Unlimited: math.IsInf(value, 1),
		UseCase:   useCase}

	return websocketClient.sendMessage(answer)
}

func (websocketClient *WebsocketClient) sendValueArr(ski string, messageType int, useCase string, values []float64) error {
	finiteValues := make([]float64, len(values))
	for i, value := range values {
		finiteValues[i] = finite(value)
	}

	answer := Message{
		SKI:     ski,
		Type:    messageType,
		Values:  finiteValues,
		UseCase: useCase}

	return websocketClient.sendMessage(answer)
}

func (websocketClient *WebsocketClient) sendLimit(ski string, messageType int, useCase string, limit ucapi.LoadLimit) error {
	unlimited := math.IsInf(limit.Value, 1)
	limit.Value = finite(limit.Value)

	answer := Message{
		SKI:       ski,
		Type:      messageType,
		Limit:     limit,
		Unlimited: unlimited,
		UseCase:   useCase}

	return websocketClient.sendMessage(answer)
}

func (websocketClient *WebsocketClient) sendServiceList(messageType int, services []shipapi.RemoteMdnsService) error {
	answer := Message{
		Type:        messageType,
		ServiceList: services}

	return websocketClient.sendMessage(answer)
}

func (websocketClient *WebsocketClient) sendEntityInfo(messageType int, remoteInfos map[string]RemoteInfo) error {
	websocketClient.mutex2.Lock()
	defer websocketClient.mutex2.Unlock()

	entityInfos := []EntityInfo{}

	for _, remoteInfo := range remoteInfos {
		device := remoteInfo.Device
		if device != nil {
			for _, entity := range device.Entities() {
				features := []string{}

				for _, f := range entity.Features() {
					features = append(features, f.String()+", "+string(f.Role()))
				}

				info := EntityInfo{
					Address:  entity.Address().String(),
					Name:     string(entity.EntityType()),
					SKI:      device.Ski(),
					Type:     string(*device.DeviceType()),
					Features: features}

				entityInfos = append(entityInfos, info)
			}
		}
	}

	answer := Message{
		Type:        messageType,
		EntityInfos: entityInfos}

	return websocketClient.sendMessage(answer)
}

func (websocketClient *WebsocketClient) sendUseCaseInfo(messageType int, useCaseInfos map[string][]UseCaseInfo) error {
	websocketClient.mutex2.Lock()
	defer websocketClient.mutex2.Unlock()

	answer := Message{
		Type:         messageType,
		UseCaseInfos: useCaseInfos}

	return websocketClient.sendMessage(answer)
}
