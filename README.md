# ControlBox

ControlBox is a sample EEBUS GridGuard implementation that simulates a control box (e.g. a smart meter gateway with control unit) and implements these EEBUS use cases:

- EnergyGuard Limitation of Power Consumption (LPC)
- EnergyGuard Limitation of Power Production (LPP)
- MonitoringAppliance Monitoring of Power Consumption (MPC)
- MonitoringAppliance Monitoring of Grid Connection Point (MGCP)

It works with devices that trust the ControlBox SKI directly (e.g. evcc) as well as with devices that only accept a control box via the SHIP Pairing Service (e.g. the Weidmüller EEBUS Relais Converter, ERC).

### Features

- **SHIP Pairing Service:** pair with devices that do not allow entering a trusted SKI, using the content of the device's pairing QR code
- **Quick buttons:** set the consumption limit (`Limit` = 4200 W, `No Limit`) and the allowed feed-in (`0 %`, `30 %`, `60 %`, `100 %` of the installed PV power) with one click
- **Output LEDs:** show which outputs (e.g. the ERC's S1, S2, W3, W4) should be active, based on the limits confirmed by the device, so the device's behavior can be compared directly
- **Unlimited values:** limits reported as unlimited (`+Inf`) by a device are shown as empty fields with the placeholder *unlimited*
- **Diagnostics:** optional debug logging and a log of the entities and use cases a remote device announces

### Installation & Execution

#### ControlBox

Run ControlBox:
```
cd /path/to/controlbox
go run . 4712
2025-04-10 16:39:14 INFO  Local SKI: A46D9C217B8F335E921C4FAA087E615C9D2A73F0
```

Note the local SKI which is logged on ControlBox startup. Certificate and key are automatically created and saved.

#### Configuration

ControlBox is configured via environment variables or a `.env` file in the working directory:

| Variable | Description |
|---|---|
| `CERT_PEM`, `KEY_PEM` | Certificate and private key as inline PEM. Generated and saved to `.env` on first run if absent. |
| `PAIRING_QR` | Content of a device's SHIP Pairing QR code (`SHIP;SKI:…;ID:…;FPH256:…;SPSEC:…;ENDSHIP;`). ControlBox then announces itself to this device via the SHIP Pairing Service. |
| `MDNS_ZEROCONF` | Set to any value to use the native Go zeroconf implementation instead of avahi for mDNS. Required for devices avahi fails to resolve, such as the Weidmüller ERC. |
| `EEBUS_DEBUG` | Set to any value to enable debug logging (SHIP handshake, SPINE messages). |

`.env` contains the private key and possibly the pairing secret. Do not commit it.

#### evcc

As of evcc 0.301.0, EEBUS is enabled by default with certificate/key being automatically created, dramatically simplifying setup.

Add ControlBox to the evcc config, e.g. `evcc.yaml`:
```
hems:
  type: eebus
  ski: A46D9C217B8F335E921C4FAA087E615C9D2A73F0 # local SKI of the ControlBox
```

Restarting evcc will automatically connect evcc to the ControlBox.

#### Weidmüller ERC and other SHIP Pairing devices

Devices like the Weidmüller EEBUS Relais Converter do not allow entering a trusted SKI. Instead, they carry a QR code containing their pairing secret (`SPSEC`). ControlBox uses it to announce itself, and the device then trusts ControlBox automatically.

1. Scan the device's QR code with any QR code app and copy its text.
2. Add it to `.env`, together with the zeroconf switch:
   ```
   PAIRING_QR="SHIP;SKI:...;ID:...;FPH256:...;SPSEC:...;ENDSHIP;"
   MDNS_ZEROCONF=1
   ```
3. Start ControlBox. On success it logs `RemoteServiceConnected` and the device's use cases.

Some devices only accept pairing for a limited time after power-up, and only trust one control box at a time.

#### ControlBox Frontend

Install dependencies:
```
cd /path/to/controlbox/frontend
npm install
```

Run web server:
```
npm run dev
```

Open ControlBox UI via web browser URI:
```
http://localhost:7081/
```

<p align="center"><img width="795" height="866" alt="image" src="https://github.com/user-attachments/assets/dc1fb9ff-2b89-4738-9e94-0a7d1f43111c" /></p>

**Quick buttons:** below the *Consumption Limit* and *Production Limit* headings. Each button fills *Active* and *Dimmed Value* and sends the limit immediately, using the *Dimmed Duration* from the form.

| Button | Active | Dimmed Value |
|---|---|---|
| `Limit` | yes | 4200 W |
| `No Limit` | no | 4200 W |
| `0 %` / `30 %` / `60 %` | yes | −(installed PV power × %) |
| `100 %` | no | −(installed PV power) |

The feed-in buttons require the *Installed PV Power [Wp]* field in the *Production Limit* panel. Feed-in limits are sent as negative values, as the ERC expects and reports them. The button matching the limit confirmed by the device is highlighted.

**Output LEDs:** the *ERC Outputs* section shows four LEDs with these defaults:

| LED | Lit when |
|---|---|
| S1 | feed-in limited to 60 % |
| S2 | feed-in limited to 30 % |
| W3 | feed-in limited to 0 % |
| W4 | consumption limit active |

For feed-in limits between steps, the highest step not exceeding the allowed feed-in is lit (e.g. 45 % lights S2). The rules can be changed under *Configure outputs*. The LEDs follow the limits confirmed by the device, not the values in the form. Failsafe states are not shown.

Installed PV power and LED rules are stored in the browser.

### Credits

ControlBox builds on the work of others:

- **[enbility](https://github.com/enbility)** – the EEBUS implementation [eebus-go](https://github.com/enbility/eebus-go), [ship-go](https://github.com/enbility/ship-go) and [spine-go](https://github.com/enbility/spine-go) by Andreas Linde, Timo Vogel and contributors, which this project is based on
- **[vollautomat](https://github.com/vollautomat/eebus-go)** – the original ControlBox implementation in their eebus-go fork
- **[andig/controlbox](https://github.com/andig/controlbox)** – the standalone ControlBox project this fork is based on, by andig, Klaus Jaroslawsky, CiNcH83 and contributors

This fork adds SHIP Pairing Service support, Weidmüller ERC compatibility, the quick buttons and the output LEDs.

Licensed under the MIT license, see [LICENSE](LICENSE).
