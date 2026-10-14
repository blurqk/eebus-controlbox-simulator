package main

import (
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/enbility/eebus-go/api"
	"github.com/enbility/eebus-go/service"
	ucapi "github.com/enbility/eebus-go/usecases/api"
	"github.com/enbility/eebus-go/usecases/eg/lpc"
	"github.com/enbility/eebus-go/usecases/eg/lpp"
	"github.com/enbility/eebus-go/usecases/ma/mgcp"
	"github.com/enbility/eebus-go/usecases/ma/mpc"
	shipapi "github.com/enbility/ship-go/api"
	"github.com/enbility/ship-go/cert"
	"github.com/enbility/ship-go/mdns"
	spineapi "github.com/enbility/spine-go/api"
	"github.com/enbility/spine-go/model"
	"github.com/joho/godotenv"
	_ "github.com/joho/godotenv/autoload"
)

type failsafeLimits struct {
	Value    float64
	Duration time.Duration
}

type controlbox struct {
	myService *service.Service

	uclpc  ucapi.EgLPCInterface
	uclpp  ucapi.EgLPPInterface
	ucmgcp ucapi.MaMGCPInterface
	ucmpc  ucapi.MaMPCInterface

	isConnected map[string]bool

	remoteInfos  map[string]RemoteInfo
	useCaseInfos map[string][]UseCaseInfo

	consumptionLimits         ucapi.LoadLimit
	productionLimits          ucapi.LoadLimit
	consumptionFailsafeLimits failsafeLimits
	productionFailsafeLimits  failsafeLimits
	consumptionNominalMax     float64
	productionNominalMax      float64

	currentRemoteServices []shipapi.RemoteMdnsService

	mutex sync.Mutex
}

// resolveCertificate loads a TLS certificate from CERT_PEM / KEY_PEM.
// OS environment variables are checked first; .env fills in any gaps.
// If neither source provides values a self-signed certificate is generated,
// persisted and returned so it is usable in the current run too.
func resolveCertificate() (tls.Certificate, error) {
	certPEM := os.Getenv("CERT_PEM")
	keyPEM := os.Getenv("KEY_PEM")
	if certPEM != "" && keyPEM != "" {
		return tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	}
	if (certPEM != "") != (keyPEM != "") {
		return tls.Certificate{}, fmt.Errorf(
			"both CERT_PEM and KEY_PEM must be set together (only one is present)")
	}

	// Nothing configured — generate and persist.
	return generateAndPersistCertificate()
}

// generateAndPersistCertificate creates a self-signed certificate, encodes it
// as inline PEM, appends CERT_PEM / KEY_PEM to .env for future runs, and
// returns the certificate directly so it is usable in the current run too.
func generateAndPersistCertificate() (tls.Certificate, error) {
	const envPath = ".env"
	log.Printf("No certificate configured — generating self-signed certificate")

	certTLS, err := cert.CreateCertificate("Demo", "Demo", "DE", "Demo-Unit-01")
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate certificate: %w", err)
	}

	// Encode certificate PEM
	certPEMBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certTLS.Certificate[0],
	})

	// Encode private key PEM
	privKey, ok := certTLS.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		return tls.Certificate{}, fmt.Errorf("unexpected private key type")
	}
	keyBytes, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("marshal private key: %w", err)
	}
	keyPEMBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})

	// Write CERT_PEM / KEY_PEM to .env — if we got here they were absent.
	// godotenv.Write handles quoting/escaping; multi-line PEM is stored as-is.
	if err := godotenv.Write(map[string]string{
		"CERT_PEM": string(certPEMBytes),
		"KEY_PEM":  string(keyPEMBytes),
	}, envPath); err != nil {
		return tls.Certificate{}, fmt.Errorf("write %s: %w", envPath, err)
	}

	log.Printf("Certificate generated and persisted to %s as CERT_PEM / KEY_PEM", envPath)
	return certTLS, nil
}

// parsePairingQR parses the content of a SHIP Pairing Service QR code, e.g.
// SHIP;SKI:xxx;ID:xxx;BRAND:xxx;...;FPH256:xxx;SPSEC:xxx;ENDSHIP;
// Devices like the Weidmueller ERC do not let the user enter a trusted SKI;
// instead the control unit has to announce itself using the secret (SPSEC)
// printed in the device's QR code.
func parsePairingQR(qr string) (shipapi.PairingTarget, error) {
	var target shipapi.PairingTarget

	qr = strings.TrimSpace(qr)
	if !strings.HasPrefix(qr, "SHIP;") || !strings.HasSuffix(qr, "ENDSHIP;") {
		return target, fmt.Errorf("pairing QR must start with 'SHIP;' and end with 'ENDSHIP;'")
	}

	for _, field := range strings.Split(qr, ";") {
		key, value, found := strings.Cut(field, ":")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		switch key {
		case "SKI":
			target.SKI = value
		case "ID":
			target.ShipID = value
		case "FPH256":
			target.Fingerprint = value
		case "SPSEC":
			secret, err := hex.DecodeString(value)
			if err != nil {
				return target, fmt.Errorf("SPSEC is not valid hex: %w", err)
			}
			if !shipapi.PairingSecret(secret).IsValidLength() {
				return target, fmt.Errorf("SPSEC must be 16 bytes (32 hex characters)")
			}
			target.Secret = secret
		}
	}

	if target.IsEmpty() {
		return target, fmt.Errorf("pairing QR must contain SKI, ID, FPH256 and SPSEC")
	}

	return target, nil
}

func (h *controlbox) run() {
	port, err := strconv.Atoi(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	certificate, err := resolveCertificate()
	if err != nil {
		log.Fatal(err)
	}

	vendorCode := "PPC"
	deviceBrand := "PPC"
	deviceModel := "SMGWplus"
	serialNumber := "123456789"
	altIdentifier := "ControlBox Simulator SN-" + serialNumber

	h.isConnected = map[string]bool{}

	// Optional SHIP Pairing Service: act as announcer (devZ) towards a device
	// whose pairing QR code content is given in PAIRING_QR.
	var pairingConfig *shipapi.PairingConfig
	var pairingTarget *shipapi.PairingTarget
	if qr := os.Getenv("PAIRING_QR"); qr != "" {
		target, err := parsePairingQR(qr)
		if err != nil {
			log.Fatal(err)
		}
		pairingTarget = &target
		pairingConfig = shipapi.NewPairingConfig(shipapi.PairingModeAnnouncer, nil)
	}

	configuration, err := api.NewConfiguration(
		vendorCode, deviceBrand, deviceModel, serialNumber,
		[]shipapi.DeviceCategoryType{shipapi.DeviceCategoryTypeGridConnectionHub},
		model.DeviceTypeTypeElectricitySupplySystem,
		[]model.EntityTypeType{model.EntityTypeTypeGridGuard},
		port, certificate, time.Second*10, pairingConfig, nil)
	if err != nil {
		log.Fatal(err)
	}
	configuration.SetAlternateIdentifier(altIdentifier)
	// Avahi fails to resolve some devices (e.g. Weidmueller ERC) that the
	// native Go zeroconf implementation handles fine.
	if os.Getenv("MDNS_ZEROCONF") != "" {
		configuration.SetMdnsProviderSelection(mdns.MdnsProviderSelectionGoZeroConfOnly)
	}

	h.myService = service.NewService(configuration, h)
	h.myService.SetLogging(h)

	if err = h.myService.Setup(); err != nil {
		fmt.Println(err)
		return
	}

	// trust every remote service, as the old SetTrusted-on-discovery did.
	// Setup creates the local service, so this must run after it.
	h.myService.SetAutoAccept(true)

	_ = h.myService.LocalDevice().Events().Subscribe(discoveryLogger{})

	localEntity := h.myService.LocalDevice().EntityForType(model.EntityTypeTypeGridGuard)
	h.uclpc = lpc.NewLPC(localEntity, h.OnLPCEvent)
	h.myService.AddUseCase(h.uclpc)

	h.uclpp = lpp.NewLPP(localEntity, h.OnLPPEvent)
	h.myService.AddUseCase(h.uclpp)

	h.ucmgcp = mgcp.NewMGCP(localEntity, h.OnMGCPEvent)
	h.myService.AddUseCase(h.ucmgcp)

	h.ucmpc = mpc.NewMPC(localEntity, h.OnMPCEvent)
	h.myService.AddUseCase(h.ucmpc)

	h.remoteInfos = map[string]RemoteInfo{}
	h.useCaseInfos = map[string][]UseCaseInfo{}

	if err := h.myService.Start(); err != nil {
		log.Fatal(err)
	}

	// Answer smgw.local and the ERC's NTP-over-TLS time request (see timeservice.go).
	if os.Getenv("SMGW_TIME") != "0" {
		startTimeService(certificate)
	}

	if pairingTarget != nil {
		// The announcer must already trust the target before announcing.
		h.myService.RegisterRemoteService(shipapi.NewServiceIdentity(
			pairingTarget.SKI, pairingTarget.Fingerprint, pairingTarget.ShipID))
		// PAIRING_ANNOUNCE=0: only connect to an already paired device, without
		// announcing. A stale announcement would let the device pair us again
		// when it looks for a replacement control box.
		if os.Getenv("PAIRING_ANNOUNCE") == "0" {
			fmt.Println("SHIP pairing: connecting to " + pairingTarget.ShipID + " without announcement")
		} else {
			if err := h.myService.StartAnnouncementTo(*pairingTarget); err != nil {
				log.Fatal("SHIP pairing announcement failed: ", err)
			}
			fmt.Println("SHIP pairing: announcing to " + pairingTarget.ShipID + " (SKI " + pairingTarget.SKI + ")")
		}
	}
}

// EEBUSServiceHandler

func (h *controlbox) RemoteServiceConnected(service api.ServiceInterface, identity shipapi.ServiceIdentity) {
	remoteSki = identity.SKI
	fmt.Println("RemoteServiceConnected: " + identity.SKI)
	h.isConnected[identity.SKI] = true

	frontend.sendText(SelectService, identity.SKI)
	frontend.sendDeviceStatus(h.deviceStatus())
}

func (h *controlbox) RemoteServiceDisconnected(service api.ServiceInterface, identity shipapi.ServiceIdentity) {
	fmt.Println("RemoteServiceDisconnected: " + identity.SKI)
	h.isConnected[identity.SKI] = false

	frontend.sendNotification("", ServiceListChanged, "")
	frontend.sendDeviceStatus(h.deviceStatus())
}

func (h *controlbox) VisibleRemoteMdnsServicesUpdated(service api.ServiceInterface, entries []shipapi.RemoteMdnsService) {
	fmt.Print("VisibleRemoteMdnsServicesUpdated, count: ")
	fmt.Println(len(entries))

	// SPINE events report SKIs in lowercase, while some devices (e.g. the
	// Weidmueller ERC) announce them in uppercase via mDNS. Normalize so the
	// service list, entity infos and frontend all use the same key.
	for i := range entries {
		entries[i].Ski = strings.ToLower(entries[i].Ski)
		fmt.Println("Remote SKI: " + entries[i].Ski)
	}

	h.currentRemoteServices = entries

	frontend.sendNotification("", ServiceListChanged, "")
}

func (h *controlbox) ServiceUpdated(identity shipapi.ServiceIdentity) {
}

func (h *controlbox) ServicePairingDetailUpdate(identity shipapi.ServiceIdentity, detail *shipapi.ConnectionStateDetail) {
	if identity.SKI == remoteSki && detail.State() == shipapi.ConnectionStateRemoteDeniedTrust {
		fmt.Println("The remote service denied trust. Exiting.")
		h.myService.CancelPairing(identity)
		h.myService.UnregisterRemoteService(identity)
		h.myService.Shutdown()
		os.Exit(1)
	}

	frontend.sendNotification("", ServiceListChanged, "")
}

func (h *controlbox) ServiceAutoTrusted(service api.ServiceInterface, identity shipapi.ServiceIdentity) {
	fmt.Println("ServiceAutoTrusted: " + identity.ShipID)
}

func (h *controlbox) ServiceAutoTrustFailed(service api.ServiceInterface, identity shipapi.ServiceIdentity, reason error) {
	fmt.Println("ServiceAutoTrustFailed: "+identity.ShipID+":", reason)
}

func (h *controlbox) ServiceAutoTrustRemoved(service api.ServiceInterface, identity shipapi.ServiceIdentity, reason string) {
	fmt.Println("ServiceAutoTrustRemoved: " + identity.ShipID + ": " + reason)
}

// deviceStatus lists the connected devices with the use cases they support,
// so the frontend can offer them as targets and show their state side by side.
func (h *controlbox) deviceStatus() []DeviceStatus {
	list := []DeviceStatus{}
	for ski, connected := range h.isConnected {
		if !connected {
			continue
		}
		status := DeviceStatus{SKI: ski, Connected: true, UseCases: h.remoteInfos[ski].UseCases}
		if indx := slices.IndexFunc(h.currentRemoteServices, func(v shipapi.RemoteMdnsService) bool { return strings.EqualFold(v.Ski, ski) }); indx >= 0 {
			service := h.currentRemoteServices[indx]
			status.Brand, status.Model, status.Type, status.ShipID = service.Brand, service.Model, service.Type, service.ShipID
		}
		list = append(list, status)
	}
	slices.SortFunc(list, func(a, b DeviceStatus) int { return strings.Compare(a.SKI, b.SKI) })
	return list
}

func resultDescription(msg model.ResultDataType) string {
	if msg.Description == nil {
		return ""
	}
	return string(*msg.Description)
}

func (h *controlbox) updateEntityInfos(ski string, device spineapi.DeviceRemoteInterface, uc string) {
	info, exists := h.remoteInfos[ski]
	if !exists {
		info = RemoteInfo{
			Device:   device,
			UseCases: []string{uc},
		}
		indx := slices.IndexFunc(h.currentRemoteServices, func(v shipapi.RemoteMdnsService) bool { return strings.EqualFold(v.Ski, ski) })
		if indx >= 0 {
			info.Service = h.currentRemoteServices[indx]
		}
		h.remoteInfos[ski] = info
	} else {
		info.Device = device
		found := slices.Contains(info.UseCases, uc)
		if !found {
			info.UseCases = append(info.UseCases, uc)
			h.remoteInfos[ski] = info
		}
	}
}

func (h *controlbox) updateUseCaseInfos(ski string, device spineapi.DeviceRemoteInterface) {
	info := []UseCaseInfo{}

	for _, uc := range device.UseCases() {
		actor := string(*uc.Actor)
		names := []string{}
		for _, ucs := range uc.UseCaseSupport {
			names = append(names, string(*ucs.UseCaseName))
		}

		info = append(info, UseCaseInfo{
			Actor: actor,
			Names: names,
		})
	}

	h.useCaseInfos[ski] = info
}

// LPC Event Handler

func (h *controlbox) sendConsumptionLimit(entity spineapi.EntityRemoteInterface) {
	ski := entity.Device().Ski()
	resultCB := func(msg model.ResultDataType, msgCounter model.MsgCounterType) {
		if *msg.ErrorNumber == model.ErrorNumberTypeNoError {
			fmt.Println("Consumption limit accepted.")
			frontend.sendNotice(ski, "Consumption limit accepted", false)
		} else {
			fmt.Println("Consumption limit rejected. Code", *msg.ErrorNumber, "Description", *msg.Description)
			frontend.sendNotice(ski, fmt.Sprintf("Consumption limit rejected (code %d: %s)", *msg.ErrorNumber, resultDescription(msg)), true)
		}
	}
	msgCounter, err := h.uclpc.WriteConsumptionLimit(entity, h.consumptionLimits, resultCB)
	if err != nil {
		fmt.Println("Failed to send consumption limit", err)
		frontend.sendNotice(ski, "Consumption limit not sent: "+err.Error(), true)
		return
	}
	fmt.Println("Sent consumption limit to", entity.Device().Ski(), "with msgCounter", msgCounter)
}

func (h *controlbox) sendConsumptionFailsafeLimit(entity spineapi.EntityRemoteInterface) {
	msgCounter, err := h.uclpc.WriteFailsafeConsumptionActivePowerLimit(entity, h.consumptionFailsafeLimits.Value)
	if err != nil {
		fmt.Println("Failed to send consumption failsafe limit", err)
		frontend.sendNotice(entity.Device().Ski(), "Consumption failsafe limit not sent: "+err.Error(), true)
		return
	}
	fmt.Println("Sent consumption failsafe limit to", entity.Device().Ski(), "with msgCounter", msgCounter)
	frontend.sendNotice(entity.Device().Ski(), "Consumption failsafe limit sent", false)
}

func (h *controlbox) sendConsumptionFailsafeDuration(entity spineapi.EntityRemoteInterface) {
	msgCounter, err := h.uclpc.WriteFailsafeDurationMinimum(entity, h.consumptionFailsafeLimits.Duration)
	if err != nil {
		fmt.Println("Failed to send consumption failsafe duration", err)
		frontend.sendNotice(entity.Device().Ski(), "Consumption failsafe duration not sent: "+err.Error(), true)
		return
	}
	fmt.Println("Sent consumption failsafe duration to", entity.Device().Ski(), "with msgCounter", msgCounter)
	frontend.sendNotice(entity.Device().Ski(), "Consumption failsafe duration sent", false)
}

func (h *controlbox) readConsumptionNominalMax(entity spineapi.EntityRemoteInterface) {
	nominal, err := h.uclpc.ConsumptionNominalMax(entity)

	if err != nil {
		fmt.Println("Failed to get consumption nominal max", err)
		return
	}

	frontend.sendValue(entity.Device().Ski(), GetConsumptionNominalMax, "LPC", nominal)
}

func (h *controlbox) OnLPCEvent(ski string, device spineapi.DeviceRemoteInterface, entity spineapi.EntityRemoteInterface, event api.EventType) {
	if entity == nil {
		return
	}

	fmt.Println("--> LPC Event: " + string(event) + " from " + ski)
	connected, exists := h.isConnected[ski]
	if !exists || !connected {
		fmt.Println("--> but not connected")
		return
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.updateEntityInfos(ski, device, "LPC")
	frontend.sendEntityInfo(GetEntityInfos, h.remoteInfos)
	h.updateUseCaseInfos(ski, device)
	frontend.sendUseCaseInfo(GetUseCaseInfos, h.useCaseInfos)
	frontend.sendDeviceStatus(h.deviceStatus())

	switch event {
	case lpc.UseCaseSupportUpdate:
		readData(h, entity, []string{"LPC"})

	case lpc.DataUpdateLimit:
		if currentLimit, err := h.uclpc.ConsumptionLimit(entity); err == nil {
			{ // every device: the frontend keeps data per SKI
				fmt.Println("Event lpc.DataUpdateLimit", ski, currentLimit.Value)

				h.consumptionLimits = currentLimit

				if currentLimit.IsActive {
					fmt.Println("New consumption limit received: active,", currentLimit.Value, "W,", currentLimit.Duration)
				} else {
					fmt.Println("New consumption limit received: inactive,", currentLimit.Value, "W,", currentLimit.Duration)
				}
				frontend.sendLimit(ski, GetConsumptionLimit, "LPC", ucapi.LoadLimit{
					IsActive: currentLimit.IsActive,
					Duration: currentLimit.Duration / time.Second,
					Value:    currentLimit.Value})
			}
		}
	case lpc.DataUpdateFailsafeConsumptionActivePowerLimit:
		if limit, err := h.uclpc.FailsafeConsumptionActivePowerLimit(entity); err == nil {
			{ // every device: the frontend keeps data per SKI
				fmt.Println("Event lpc.DataUpdateFailsafeConsumptionActivePowerLimit", ski, limit)

				h.consumptionFailsafeLimits.Value = limit

				frontend.sendValue(ski, GetConsumptionFailsafeValue, "LPC", limit)
			}
		}
	case lpc.DataUpdateFailsafeDurationMinimum:
		if duration, err := h.uclpc.FailsafeDurationMinimum(entity); err == nil {
			{ // every device: the frontend keeps data per SKI
				fmt.Println("Event lpc.DataUpdateFailsafeDurationMinimum", ski, duration)

				h.consumptionFailsafeLimits.Duration = duration

				frontend.sendValue(ski, GetConsumptionFailsafeDuration, "LPC", float64(duration/time.Second))
			}
		}
		// TODO
	// case lpc.DataUpdateHeartbeat:
	// 	if ski == remoteSki {
	// 		h.readConsumptionNominalMax(entity)
	// 		frontend.sendNotification(ski, GetConsumptionHeartbeat, "LPC")
	// 	}
	default:
		return
	}
}

// LPP Event Handler

func (h *controlbox) sendProductionLimit(entity spineapi.EntityRemoteInterface) {
	ski := entity.Device().Ski()
	resultCB := func(msg model.ResultDataType, msgCounter model.MsgCounterType) {
		if *msg.ErrorNumber == model.ErrorNumberTypeNoError {
			fmt.Println("Production limit accepted.")
			frontend.sendNotice(ski, "Production limit accepted", false)
		} else {
			fmt.Println("Production limit rejected. Code", *msg.ErrorNumber, "Description", *msg.Description)
			frontend.sendNotice(ski, fmt.Sprintf("Production limit rejected (code %d: %s)", *msg.ErrorNumber, resultDescription(msg)), true)
		}
	}
	msgCounter, err := h.uclpp.WriteProductionLimit(entity, h.productionLimits, resultCB)
	if err != nil {
		fmt.Println("Failed to send production limit", err)
		frontend.sendNotice(ski, "Production limit not sent: "+err.Error(), true)
		return
	}
	fmt.Println("Sent production limit to", entity.Device().Ski(), "with msgCounter", msgCounter)
}

func (h *controlbox) sendProductionFailsafeLimit(entity spineapi.EntityRemoteInterface) {
	msgCounter, err := h.uclpp.WriteFailsafeProductionActivePowerLimit(entity, h.productionFailsafeLimits.Value)
	if err != nil {
		fmt.Println("Failed to send production failsafe limit", err)
		frontend.sendNotice(entity.Device().Ski(), "Production failsafe limit not sent: "+err.Error(), true)
		return
	}
	fmt.Println("Sent production failsafe limit to", entity.Device().Ski(), "with msgCounter", msgCounter)
	frontend.sendNotice(entity.Device().Ski(), "Production failsafe limit sent", false)
}

func (h *controlbox) sendProductionFailsafeDuration(entity spineapi.EntityRemoteInterface) {
	msgCounter, err := h.uclpp.WriteFailsafeDurationMinimum(entity, h.productionFailsafeLimits.Duration)
	if err != nil {
		fmt.Println("Failed to send production failsafe duration", err)
		frontend.sendNotice(entity.Device().Ski(), "Production failsafe duration not sent: "+err.Error(), true)
		return
	}
	fmt.Println("Sent production failsafe duration to", entity.Device().Ski(), "with msgCounter", msgCounter)
	frontend.sendNotice(entity.Device().Ski(), "Production failsafe duration sent", false)
}

func (h *controlbox) readProductionNominalMax(entity spineapi.EntityRemoteInterface) {
	nominal, err := h.uclpp.ProductionNominalMax(entity)

	if err != nil {
		fmt.Println("Failed to get production nominal max", err)
		return
	}

	frontend.sendValue(entity.Device().Ski(), GetProductionNominalMax, "LPP", nominal)
}

func (h *controlbox) OnLPPEvent(ski string, device spineapi.DeviceRemoteInterface, entity spineapi.EntityRemoteInterface, event api.EventType) {
	if entity == nil {
		return
	}

	fmt.Println("--> LPP Event: " + string(event) + " from " + ski)
	connected, exists := h.isConnected[ski]
	if !exists || !connected {
		fmt.Println("--> but not connected")
		return
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.updateEntityInfos(ski, device, "LPP")
	frontend.sendEntityInfo(GetEntityInfos, h.remoteInfos)
	h.updateUseCaseInfos(ski, device)
	frontend.sendUseCaseInfo(GetUseCaseInfos, h.useCaseInfos)
	frontend.sendDeviceStatus(h.deviceStatus())

	switch event {
	case lpp.UseCaseSupportUpdate:
		readData(h, entity, []string{"LPP"})

	case lpp.DataUpdateLimit:
		if currentLimit, err := h.uclpp.ProductionLimit(entity); err == nil {
			{ // every device: the frontend keeps data per SKI
				fmt.Println("Event lpp.DataUpdateLimit", ski, currentLimit.Value)

				h.productionLimits = currentLimit

				if currentLimit.IsActive {
					fmt.Println("New production limit received: active,", currentLimit.Value, "W,", currentLimit.Duration)
				} else {
					fmt.Println("New production limit received: inactive,", currentLimit.Value, "W,", currentLimit.Duration)
				}

				frontend.sendLimit(ski, GetProductionLimit, "LPP", ucapi.LoadLimit{
					IsActive: currentLimit.IsActive,
					Duration: currentLimit.Duration / time.Second,
					Value:    currentLimit.Value})
			}
		}
	case lpp.DataUpdateFailsafeProductionActivePowerLimit:
		if limit, err := h.uclpp.FailsafeProductionActivePowerLimit(entity); err == nil {
			{ // every device: the frontend keeps data per SKI
				fmt.Println("Event lpp.DataUpdateFailsafeProductionActivePowerLimit", ski, limit)

				h.productionFailsafeLimits.Value = limit

				frontend.sendValue(ski, GetProductionFailsafeValue, "LPP", limit)
			}
		}
	case lpp.DataUpdateFailsafeDurationMinimum:
		if duration, err := h.uclpp.FailsafeDurationMinimum(entity); err == nil {
			{ // every device: the frontend keeps data per SKI
				fmt.Println("Event lpp.DataUpdateFailsafeDurationMinimum", ski, duration)

				h.productionFailsafeLimits.Duration = duration

				frontend.sendValue(ski, GetProductionFailsafeDuration, "LPP", float64(duration/time.Second))
			}
		}
		// TODO
	// case lpp.DataUpdateHeartbeat:
	// 	if ski == remoteSki {
	// 		h.readProductionNominalMax(entity)
	// 		frontend.sendNotification(ski, GetProductionHeartbeat, "LPP")
	// 	}
	default:
		return
	}
}

func (h *controlbox) OnMGCPEvent(ski string, device spineapi.DeviceRemoteInterface, entity spineapi.EntityRemoteInterface, event api.EventType) {
	if entity == nil {
		return
	}

	fmt.Println("--> MGCP Event: " + string(event) + " from " + ski)
	connected, exists := h.isConnected[ski]
	if !exists || !connected {
		fmt.Println("--> but not connected")
		return
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.updateEntityInfos(ski, device, "MGCP")
	frontend.sendEntityInfo(GetEntityInfos, h.remoteInfos)
	h.updateUseCaseInfos(ski, device)
	frontend.sendUseCaseInfo(GetUseCaseInfos, h.useCaseInfos)
	frontend.sendDeviceStatus(h.deviceStatus())

	switch event {
	case mgcp.UseCaseSupportUpdate:
		readData(h, entity, []string{"MGCP"})

	case mgcp.DataUpdatePowerLimitationFactor:
		if powerLimitFactor, err := h.ucmgcp.PowerLimitationFactor(entity); err == nil {
			frontend.sendValue(ski, GetPowerLimitationFactor, "MGCP", powerLimitFactor)
		}
	case mgcp.DataUpdatePower:
		if power, err := h.ucmgcp.Power(entity); err == nil {
			frontend.sendValue(ski, GetPower, "MGCP", power)
		}
	case mgcp.DataUpdateEnergyFeedIn:
		if energyFeedIn, err := h.ucmgcp.EnergyFeedIn(entity); err == nil {
			frontend.sendValue(ski, GetEnergyFeedIn, "MGCP", energyFeedIn)
		}
	case mgcp.DataUpdateEnergyConsumed:
		if energyConsumed, err := h.ucmgcp.EnergyConsumed(entity); err == nil {
			frontend.sendValue(ski, GetEnergyConsumed, "MGCP", energyConsumed)
		}
	case mgcp.DataUpdateCurrentPerPhase:
		if currentPerPhase, err := h.ucmgcp.CurrentPerPhase(entity); err == nil {
			frontend.sendValueArr(ski, GetCurrentPerPhase, "MGCP", currentPerPhase)
		}
	case mgcp.DataUpdateVoltagePerPhase:
		if voltagePerPhase, err := h.ucmgcp.VoltagePerPhase(entity); err == nil {
			frontend.sendValueArr(ski, GetVoltagePerPhase, "MGCP", voltagePerPhase)
		}
	case mgcp.DataUpdateFrequency:
		if frequency, err := h.ucmgcp.Frequency(entity); err == nil {
			frontend.sendValue(ski, GetFrequency, "MGCP", frequency)
		}
	}
}

func (h *controlbox) OnMPCEvent(ski string, device spineapi.DeviceRemoteInterface, entity spineapi.EntityRemoteInterface, event api.EventType) {
	if entity == nil {
		return
	}

	fmt.Println("--> MPC Event: " + string(event) + " from " + ski)
	connected, exists := h.isConnected[ski]
	if !exists || !connected {
		fmt.Println("--> but not connected")
		return
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.updateEntityInfos(ski, device, "MPC")
	frontend.sendEntityInfo(GetEntityInfos, h.remoteInfos)
	h.updateUseCaseInfos(ski, device)
	frontend.sendUseCaseInfo(GetUseCaseInfos, h.useCaseInfos)
	frontend.sendDeviceStatus(h.deviceStatus())

	switch event {
	case mpc.UseCaseSupportUpdate:
		readData(h, entity, []string{"MPC"})

	case mpc.DataUpdatePower:
		if power, err := h.ucmpc.Power(entity); err == nil {
			frontend.sendValue(ski, GetPower, "MPC", power)
		}
	case mpc.DataUpdatePowerPerPhase:
		if powerPerPhase, err := h.ucmpc.PowerPerPhase(entity); err == nil {
			frontend.sendValueArr(ski, GetPowerPerPhase, "MPC", powerPerPhase)
		}
	case mpc.DataUpdateEnergyConsumed:
		if energyConsumed, err := h.ucmpc.EnergyConsumed(entity); err == nil {
			frontend.sendValue(ski, GetEnergyConsumed, "MPC", energyConsumed)
		}
	case mpc.DataUpdateEnergyProduced:
		if energyFeedIn, err := h.ucmpc.EnergyProduced(entity); err == nil {
			frontend.sendValue(ski, GetEnergyFeedIn, "MPC", energyFeedIn)
		}
	case mpc.DataUpdateCurrentsPerPhase:
		if currentPerPhase, err := h.ucmpc.CurrentPerPhase(entity); err == nil {
			frontend.sendValueArr(ski, GetCurrentPerPhase, "MPC", currentPerPhase)
		}
	case mpc.DataUpdateVoltagePerPhase:
		if voltagePerPhase, err := h.ucmpc.VoltagePerPhase(entity); err == nil {
			frontend.sendValueArr(ski, GetVoltagePerPhase, "MPC", voltagePerPhase)
		}
	case mpc.DataUpdateFrequency:
		if frequency, err := h.ucmpc.Frequency(entity); err == nil {
			frontend.sendValue(ski, GetFrequency, "MPC", frequency)
		}
	}
}
