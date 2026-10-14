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
- **Time service for the ERC:** answers `smgw.local` and the ERC's time request, so its clock and log show the real time instead of 1970
- **Diagnostics:** optional debug logging and a log of the entities and use cases a remote device announces

### Installation & Execution

#### ControlBox

Start ControlBox, frontend and ERC time service with one command:
```
cd /path/to/controlbox
./start.sh            # SHIP port 4712; ./start.sh <port> for another one
2025-04-10 16:39:14 INFO  Local SKI: A46D9C217B8F335E921C4FAA087E615C9D2A73F0
Frontend: http://localhost:7080/
```

`start.sh` builds the frontend (`npm install`, `npm run build`) when it is missing or its sources changed, then runs `go run . 4712`. ControlBox serves the built frontend itself on port 7080, so no separate web server is needed. Requires Go and Node.js.

Note the local SKI which is logged on ControlBox startup. Certificate and key are automatically created and saved.

#### Configuration

ControlBox is configured via environment variables or a `.env` file in the working directory:

| Variable | Description |
|---|---|
| `CERT_PEM`, `KEY_PEM` | Certificate and private key as inline PEM. Generated and saved to `.env` on first run if absent. |
| `PAIRING_QR` | Content of a device's SHIP Pairing QR code (`SHIP;SKI:…;ID:…;FPH256:…;SPSEC:…;ENDSHIP;`). ControlBox then announces itself to this device via the SHIP Pairing Service. |
| `MDNS_ZEROCONF` | Set to any value to use the native Go zeroconf implementation instead of avahi for mDNS. Required for devices avahi fails to resolve, such as the Weidmüller ERC. |
| `SMGW_TIME` | Time service for the Weidmüller ERC (see below), on by default. `0` switches it off. |
| `SMGW_IP` | IPv4 address announced for `smgw.local`. Default: the address of the interface the query arrived on. |
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

#### Time for the Weidmüller ERC

The ERC gets its clock from the PPC smart meter gateway it was designed for, not from the internet. At boot it looks up `smgw.local` via mDNS, opens a TLS 1.2 connection to TCP port 4460 and sends a plain 48 byte NTP client packet inside it. It does not verify the server certificate. Without an answer it logs `PPCCLS_TIMESYNC_CONNECT: could not connect to peer` and its clock starts at 1970.

ControlBox answers both the mDNS query and the time request with the host's clock (UTC). The ERC asks only once per boot, so restart it after starting ControlBox. Only one device should answer `smgw.local`: run either this simulator or the Tasmota one, not both (`SMGW_TIME=0` here or `EEBusSmgw 0` on the Tasmota). Binding port 4460 and sharing mDNS port 5353 needs no root rights. The ERC must be able to reach TCP port 4460 on the ControlBox host.

#### ControlBox Frontend

`start.sh` builds the frontend and ControlBox serves it. Open the UI at:
```
http://localhost:7080/
```

For frontend development, the Vite dev server with hot reload still works (UI at `http://localhost:7081/`):
```
cd /path/to/controlbox/frontend
npm install
npm run dev
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
