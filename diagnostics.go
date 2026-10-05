package main

import (
	"fmt"

	spineapi "github.com/enbility/spine-go/api"
	"github.com/enbility/spine-go/model"
)

// discoveryLogger prints what a remote device announces via SPINE: its
// entities after detailed discovery and the use cases it claims to support.
// Useful when a connected device does not show up with any actors, e.g.
// because its entity type or actor does not match what LPC/LPP expect.
type discoveryLogger struct{}

func (discoveryLogger) HandleEvent(payload spineapi.EventPayload) {
	switch payload.EventType {
	case spineapi.EventTypeDeviceChange, spineapi.EventTypeEntityChange:
		if payload.Device == nil || payload.ChangeType != spineapi.ElementChangeAdd {
			return
		}
		fmt.Println("--> Discovery from", payload.Ski)
		for _, entity := range payload.Device.Entities() {
			fmt.Printf("    entity %v type %s\n", entity.Address().Entity, entity.EntityType())
			for _, feature := range entity.Features() {
				fmt.Printf("        feature %s (%s)\n", feature.Type(), feature.Role())
			}
		}
	case spineapi.EventTypeDataChange:
		data, ok := payload.Data.(*model.NodeManagementUseCaseDataType)
		if !ok || data == nil {
			return
		}
		fmt.Println("--> Use cases from", payload.Ski)
		for _, uc := range data.UseCaseInformation {
			var actor model.UseCaseActorType
			if uc.Actor != nil {
				actor = *uc.Actor
			}
			var entity []model.AddressEntityType
			if uc.Address != nil {
				entity = uc.Address.Entity
			}
			for _, support := range uc.UseCaseSupport {
				if support.UseCaseName != nil {
					fmt.Printf("    entity %v actor %s use case %s\n", entity, actor, *support.UseCaseName)
				}
			}
		}
	}
}
