<template>
  <h1>
    <span>Control Box Simulator</span>
    <nav class="pages">
      <button v-for="p in pages" :key="p.id" :class="{ active: page == p.id }" @click="showPage( p.id )">{{ p.label }}</button>
    </nav>
  </h1>

  <div class="wrap">
    <div v-if="'' == qrcode" class="card">
      <h2>Not running</h2>
      <p class="hint">No connection to the ControlBox backend.</p>
    </div>

    <!-- ===== CONTROL ===== -->
    <template v-if="'control' == page && '' < qrcode">
      <div class="card">
        <div class="devbar">
          <span class="hint inline">Target</span>
          <select v-model="target" @change="saveSettings">
            <option value="all">All connected devices</option>
            <option v-for="d in devices" :key="d.SKI" :value="d.SKI">{{ deviceName( d.SKI ) }}</option>
          </select>
          <span :class="['pill', 0 < targets.length ? 'ok' : 'bad']">
            {{ 0 < targets.length ? targets.length + ( 1 == targets.length ? ' device' : ' devices' ) : 'not connected' }}
          </span>
        </div>
      </div>

      <div class="duo">
        <section class="card">
          <h2>§ 14a EnWG — Consumption (LPC)</h2>
          <p v-if="! targetsWith( 'LPC' ).length" class="hint">No device in the target supports LPC.</p>
          <div class="quick fill">
            <button :class="{ sel: allConfirm( 'LPC', l => l.IsActive && l.Value == consumptionLimitValue ) }"
              :disabled="! targetsWith( 'LPC' ).length"
              :title="'Limit consumption to ' + formatted( consumptionLimitValue ) + ' W'"
              @click="setConsumption( true )">Limit · {{ formatted( consumptionLimitValue ) }} W</button>
            <button :class="{ sel: allConfirm( 'LPC', l => ! l.IsActive ) }"
              :disabled="! targetsWith( 'LPC' ).length"
              title="Release the consumption limit"
              @click="setConsumption( false )">No Limit</button>
          </div>
          <div class="row2"><label>Limit [W]</label><input type="number" v-model.number="form.lpcValue" placeholder="e.g. 4200" @change="saveSettings" /></div>
          <div class="row2"><label>Duration [s]</label><input type="number" v-model.number="form.lpcDuration" min="0" max="86400" step="60" @change="saveSettings" /></div>
          <div class="quick center">
            <button :disabled="! targetsWith( 'LPC' ).length || isEmpty( form.lpcValue )" @click="sendConsumptionLimit( true )">Set limit</button>
            <button class="grey" :disabled="! targetsWith( 'LPC' ).length" @click="sendConsumptionLimit( false )">Release</button>
          </div>
        </section>

        <section class="card">
          <h2>§ 9 EEG — Feed-in (LPP)</h2>
          <p v-if="! targetsWith( 'LPP' ).length" class="hint">No device in the target supports LPP.</p>
          <div class="quick fill">
            <button v-for="pct in feedInSteps" :key="pct"
              :class="{ sel: allConfirm( 'LPP', l => isFeedInStep( l, pct ) ) }"
              :disabled="! targetsWith( 'LPP' ).length || ( pct < 100 && ! pvWp )"
              :title="feedInHint( pct )"
              @click="setFeedIn( pct )">{{ pct }} %</button>
          </div>
          <div class="row2"><label>Limit [W]</label><input type="number" v-model.number="form.lppValue" placeholder="amount" @change="saveSettings" /></div>
          <div class="row2"><label>Duration [s]</label><input type="number" v-model.number="form.lppDuration" min="0" max="86400" step="60" @change="saveSettings" /></div>
          <div class="quick center">
            <button :disabled="! targetsWith( 'LPP' ).length || isEmpty( form.lppValue )" @click="sendProductionLimit( true )">Set limit</button>
            <button class="grey" :disabled="! targetsWith( 'LPP' ).length" @click="sendProductionLimit( false )">Release</button>
          </div>
        </section>
      </div>
      <p class="hint below">Duration: 600 s = 10 minutes · 0 = unlimited. Feed-in as an amount — it is sent as a negative value, as the devices expect and report it.</p>

      <div class="card">
        <h2>Status</h2>
        <table v-if="targets.length" class="tab">
          <tr><th>Device</th><th>Consumption § 14a</th><th>Feed-in § 9</th><th>Outputs</th></tr>
          <tr v-for="d in targets" :key="d.SKI">
            <td>{{ deviceName( d.SKI ) }}</td>
            <td class="mono">{{ supports( d, 'LPC' ) ? limitText( d.SKI, 'LPC' ) : 'not supported' }}</td>
            <td class="mono">{{ supports( d, 'LPP' ) ? limitText( d.SKI, 'LPP' ) : 'not supported' }}</td>
            <td>
              <span v-for="rule in ledRules" :key="rule.Name" class="ledi" :title="ledHint( rule )">
                <span :class="['led', ledState( rule, d.SKI )]"></span>{{ rule.Name }}
              </span>
            </td>
          </tr>
        </table>
        <p v-else class="hint">No connected device in the target.</p>
        <p v-if="ledRules.some( r => 'LPP' == r.Source ) && ! pvWp" class="hint">
          Feed-in outputs need the installed PV power (Settings → Installation).</p>
      </div>
    </template>

    <!-- ===== CONNECTION ===== -->
    <template v-if="'connection' == page && '' < qrcode">
      <div class="card">
        <h2>Own Identity</h2>
        <div class="identity">
          <qrcode-vue class="qrcode" :value="qrcode" :size="130" level="H" render-as="svg" />
          <span class="mono">{{ qrcode }}</span>
        </div>
        <p class="hint">Enter the SKI from this text at the device or in its app, or scan the QR code.</p>
      </div>

      <div class="card">
        <h2>Device</h2>
        <div class="row">
          <label>Found devices</label>
          <select v-model="selectedSki" @change="serviceSelected">
            <option value="">{{ remoteServices.length ? '— select —' : 'No devices found' }}</option>
            <option v-for="rs in remoteServices" :key="rs.ski" :value="rs.ski">{{ serviceLabel( rs ) }}</option>
          </select>
          <span :class="['pill', isConnected( selectedSki ) ? 'ok' : 'bad']">{{ isConnected( selectedSki ) ? 'connected' : 'not connected' }}</span>
        </div>
        <div v-if="selectedService" class="ident">
          <b>SKI</b><span class="mono">{{ readableSKI( selectedSki ) }}</span>
          <b>ID</b><span>{{ selectedService.shipID }}</span>
          <b>Brand</b><span>{{ selectedService.brand }}</span>
          <b>Type</b><span>{{ selectedService.type }}</span>
          <b>Model</b><span>{{ selectedService.model }}</span>
          <b>Serial</b><span>{{ selectedService.serial }}</span>
          <b>Nominal max</b><span>{{ nominalText( selectedSki ) }}</span>
        </div>
        <p v-if="selectedService && isHems( selectedService ) && ! isConnected( selectedSki )" class="hint">
          Energy managers connect to the control box themselves.</p>
      </div>

      <div v-if="'' < selectedSki" class="card">
        <h2>Structure &amp; Measurements</h2>
        <div class="grid2">
          <div>
            <h3>Structure</h3>
            <div class="row"><label>Actors</label>
              <select v-model="selectedActor"><option :value="undefined">{{ countText( actorList.length, 'actor' ) }}</option>
                <option v-for="a in actorList" :key="a.Actor" :value="a">{{ a.Actor }}</option></select></div>
            <div class="row"><label>Use cases</label>
              <select><option>{{ selectedActor ? countText( selectedActor.Names.length, 'use case' ) : '—' }}</option>
                <option v-for="n in selectedActor?.Names ?? []" :key="n">{{ n }}</option></select></div>
            <div class="row"><label>Entities</label>
              <select v-model="selectedEntity"><option :value="undefined">{{ countText( selectedEntities.length, 'entity', 'entities' ) }}</option>
                <option v-for="e in selectedEntities" :key="e.Address" :value="e">{{ e.Name }}</option></select></div>
            <div class="row"><label>Features</label>
              <select><option>{{ selectedEntity ? countText( selectedEntity.Features.length, 'feature' ) : '—' }}</option>
                <option v-for="f in selectedEntity?.Features ?? []" :key="f">{{ f }}</option></select></div>
          </div>
          <div>
            <template v-for="uc in ['MGCP', 'MPC']" :key="uc">
              <template v-if="monitorings[selectedSki]?.[uc]">
                <h3>{{ 'MGCP' == uc ? 'Grid connection point · MGCP' : 'Power consumption · MPC' }}</h3>
                <div class="info">
                  <template v-if="'MGCP' == uc"><b>Power limitation factor</b><span>{{ monitorings[selectedSki][uc].PowerLimitationFactor ?? '—' }} %</span></template>
                  <b>Power</b><span>{{ formatted( monitorings[selectedSki][uc].Power ?? 0 ) }} W</span>
                  <template v-if="'MPC' == uc"><b>Power L1/L2/L3</b><span>{{ phases( monitorings[selectedSki][uc].PowerPerPhase ) }} W</span></template>
                  <b>Energy consumed</b><span>{{ formatted( monitorings[selectedSki][uc].EnergyConsumed ?? 0 ) }} Wh</span>
                  <b>Energy fed in</b><span>{{ formatted( monitorings[selectedSki][uc].EnergyFeedIn ?? 0 ) }} Wh</span>
                  <b>Currents L1/L2/L3</b><span>{{ phases( monitorings[selectedSki][uc].CurrentPerPhase ) }} A</span>
                  <b>Voltages L1/L2/L3</b><span>{{ phases( monitorings[selectedSki][uc].VoltagePerPhase ) }} V</span>
                  <b>Frequency</b><span>{{ monitorings[selectedSki][uc].Frequency ?? '—' }} Hz</span>
                </div>
              </template>
            </template>
            <p v-if="! monitorings[selectedSki]" class="hint">The device reports no measurements (MGCP, MPC).</p>
          </div>
        </div>
      </div>
    </template>

    <!-- ===== SETTINGS ===== -->
    <template v-if="'settings' == page && '' < qrcode">
      <div class="card">
        <h2>Installation</h2>
        <div class="row">
          <label>Installed PV power [Wp]</label>
          <input type="number" v-model.number="pvWp" min="0" step="100" placeholder="Wp" @change="saveSettings" />
          <span class="hint inline">Basis of the % buttons and the feed-in outputs</span>
        </div>
      </div>

      <div class="card">
        <h2>Failsafe</h2>
        <p class="hint first">Applies when the connection to the control box is lost. Written to the target of the Control page: <b>{{ targetName }}</b>.</p>
        <table v-if="devices.length" class="tab">
          <tr><th>Device</th><th>Consumption</th><th>Feed-in</th><th>Minimum duration</th></tr>
          <tr v-for="d in devices" :key="d.SKI">
            <td>{{ deviceName( d.SKI ) }}</td>
            <td class="mono">{{ supports( d, 'LPC' ) ? wattText( failsafe[d.SKI]?.LPC?.Value ) : 'not supported' }}</td>
            <td class="mono">{{ supports( d, 'LPP' ) ? wattText( failsafe[d.SKI]?.LPP?.Value ) : 'not supported' }}</td>
            <td class="mono">{{ durationText( failsafe[d.SKI]?.LPC?.Duration ?? failsafe[d.SKI]?.LPP?.Duration ) }}</td>
          </tr>
        </table>
        <div class="row">
          <label>Failsafe consumption [W]</label>
          <input type="number" v-model.number="form.fsLpc" min="0" step="100" placeholder="e.g. 4200" @change="saveSettings" />
          <button class="sec" :disabled="! targetsWith( 'LPC' ).length || isEmpty( form.fsLpc )" @click="sendFailsafeValue( 'LPC' )">Set</button>
        </div>
        <div class="row">
          <label>Failsafe feed-in [W]</label>
          <input type="number" v-model.number="form.fsLpp" min="0" step="100" placeholder="amount" @change="saveSettings" />
          <button class="sec" :disabled="! targetsWith( 'LPP' ).length || isEmpty( form.fsLpp )" @click="sendFailsafeValue( 'LPP' )">Set</button>
        </div>
        <div class="row">
          <label>Minimum duration [s]</label>
          <input type="number" v-model.number="form.fsDuration" min="0" max="86400" step="600" placeholder="7200" @change="saveSettings" />
          <button class="sec" :disabled="! targets.length || isEmpty( form.fsDuration )" @click="sendFailsafeDuration">Set</button>
        </div>
        <p class="hint">The minimum duration applies to consumption and feed-in; the standard allows 7200–86400 s (2–24 h). Not every device supports every value (a wallbox e.g. has no feed-in failsafe).</p>
      </div>

      <div class="card">
        <h2>Appearance</h2>
        <h3>Colour scheme</h3>
        <p class="hint first">Title, headings, buttons and active page.</p>
        <div class="swatches">
          <button v-for="c in colours" :key="c" :class="['swatch', { active: c == colour.hl }]" :style="{ background: c }" :title="c" @click="setColour( c, 'hl' )"></button>
        </div>
        <h3 class="spaced">Highlighting</h3>
        <p class="hint first">Frame around the quick buttons under § 14a and § 9 whose state the devices confirmed.</p>
        <div class="swatches">
          <button v-for="c in colours" :key="c" :class="['swatch', { active: c == colour.sel }]" :style="{ background: c }" :title="c" @click="setColour( c, 'sel' )"></button>
        </div>
        <p class="hint">Both settings are stored in the browser.</p>
      </div>

      <div class="card">
        <h2>Output Assignment</h2>
        <p class="hint first">Which output (LED) is lit for which limit. The LEDs are shown under Control → Status.</p>
        <div class="ledcfg">
          <template v-for="rule in ledRules" :key="rule.Name">
            <b>{{ rule.Name }}</b>
            <select v-model="rule.Source" @change="saveSettings">
              <option value="LPP">Feed-in limited to</option>
              <option value="LPC">Consumption limit active</option>
              <option value="off">Not used</option>
            </select>
            <span v-if="'LPP' == rule.Source"><input type="number" v-model.number="rule.Step" min="0" max="100" @change="saveSettings" /> %</span>
            <span v-else></span>
          </template>
        </div>
        <p class="hint">For feed-in limits between steps, the highest step not exceeding the allowed feed-in is lit (45 % → 30 %). 100 % or no active limit: no feed-in output. Failsafe states are not shown.</p>
      </div>
    </template>

    <!-- ===== MESSAGES ===== -->
    <div v-if="'' < qrcode" class="card">
      <div class="cardhead">
        <h2>Messages</h2>
        <button class="sec icon" title="Clear messages" aria-label="Clear messages" @click="messages = []">
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2"
               stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18"/><path d="M8 6V4h8v2"/>
            <path d="M19 6l-1 14H6L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/></svg>
        </button>
      </div>
      <table v-if="messages.length" class="tab msgtab">
        <tr><th>Kind</th><th>Time</th><th>Device</th><th>Message</th></tr>
        <tr v-for="(m, i) in messages" :key="i">
          <td><span :class="['tag', m.Error ? 'error' : 'info']">{{ m.Error ? 'error' : 'info' }}</span></td>
          <td class="mono">{{ m.Time }}</td>
          <td>{{ m.Device }}</td>
          <td>{{ m.Text }}</td>
        </tr>
      </table>
      <p v-else class="hint first">No messages.</p>
    </div>
  </div>
</template>

<script lang="ts">
  import { Component, Vue, toNative } from 'vue-facing-decorator'
  import QrcodeVue from 'qrcode.vue'

  enum MessageType {
    Text                           = 0,
    QRCode                         = 1,
    Acknowledge                    = 2,
    ServiceListChanged             = 3,
    GetServiceList                 = 4,
    SelectService                  = 5,
    GetEntityInfos                 = 6,
    GetUseCaseInfos                = 7,
    GetAllData                     = 8,
    SetConsumptionLimit            = 9,
    GetConsumptionLimit            = 10,
    SetProductionLimit             = 11,
    GetProductionLimit             = 12,
    SetConsumptionFailsafeValue    = 13,
    GetConsumptionFailsafeValue    = 14,
    SetConsumptionFailsafeDuration = 15,
    GetConsumptionFailsafeDuration = 16,
    SetProductionFailsafeValue     = 17,
    GetProductionFailsafeValue     = 18,
    SetProductionFailsafeDuration  = 19,
    GetProductionFailsafeDuration  = 20,
    GetConsumptionNominalMax       = 21,
    GetProductionNominalMax        = 22,
    GetConsumptionHeartbeat        = 23,
    StopConsumptionHeartbeat       = 24,
    StartConsumptionHeartbeat      = 25,
    GetProductionHeartbeat         = 26,
    StopProductionHeartbeat        = 27,
    StartProductionHeartbeat       = 28,
    GetPowerLimitationFactor       = 29,
    GetPower                       = 30,
    GetPowerPerPhase               = 31,
    GetEnergyFeedIn                = 32,
    GetEnergyConsumed              = 33,
    GetCurrentPerPhase             = 34,
    GetVoltagePerPhase             = 35,
    GetFrequency                   = 36,
    DeviceStatusList               = 37,
    Notice                         = 38
  }

  interface LoadLimit {
    IsActive: boolean,
    Value:    number,
    Duration: number
  }

  // limit as confirmed by the device; Value null: no limit (device reported +Inf)
  interface ReportedLimit {
    IsActive: boolean,
    Value:    number | null,
    Duration: number
  }

  interface FailsafeValues {
    Value?:    number | null,
    Duration?: number
  }

  // An ERC output LED: lit for a feed-in step (LPP, in % of installed PV
  // power) or while a consumption limit (LPC) is active.
  interface LedRule {
    Name:   string,
    Source: 'LPP' | 'LPC' | 'off',
    Step:   number
  }

  interface Form {
    lpcValue:    number | string,
    lpcDuration: number | string,
    lppValue:    number | string,
    lppDuration: number | string,
    fsLpc:       number | string,
    fsLpp:       number | string,
    fsDuration:  number | string
  }

  interface Settings {
    installedPv?: {[key:string]:number | string},   // per device, before the installation setting
    pvWp?:        number | string,
    ledRules?:    LedRule[],
    target?:      string,
    form?:        Form,
    colours?:     {hl:string, sel:string}
  }

  interface Monitorings {
    PowerLimitationFactor?: number,
    Power?:                 number,
    PowerPerPhase?:         number[],
    EnergyFeedIn?:          number,
    EnergyConsumed?:        number,
    CurrentPerPhase?:       number[],
    VoltagePerPhase?:       number[],
    Frequency?:             number
  }

  interface RemoteService {
    name:       string,
    ski:        string,
    shipID:     string,
    brand:      string,
    type:       string,
    model:      string,
    serial:     string,
    categories: number[]
  }

  interface DeviceStatus {
    SKI:       string,
    Connected: boolean,
    UseCases:  string[] | null,
    Brand:     string,
    Model:     string,
    Type:      string,
    ShipID:    string
  }

  interface EntityInfo {
    Address:  string,
    Name:     string,
    SKI:      string,
    Type:     string,
    Features: string[]
  }

  interface UseCaseInfo {
    Actor: string,
    Names: string[]
  }

  type UseCaseInfos = {[key:string]:UseCaseInfo[]}

  interface Message {
    SKI:           string,
    Type:          MessageType,
    Text?:         string,
    Limit?:        LoadLimit,
    Value?:        number,
    Unlimited?:    boolean,
    Values?:       number[],
    ServiceList?:  RemoteService[],
    EntityInfos?:  EntityInfo[],
    UseCaseInfos?: UseCaseInfos,
    UseCase?:      string,
    Devices?:      DeviceStatus[],
    Error?:        boolean
  }

  interface Notice {
    Time:   string,
    Device: string,
    Text:   string,
    Error:  boolean
  }

  const settingsKey = "controlbox.settings";

  const colourList = ['#f28c28','#e05a4a','#e8558a','#d65db1','#9b6ae0','#7c5cff','#5b6ee1','#2f9be0',
                      '#22b8d8','#1fb5a5','#37c871','#8ccf3f','#e9c53a','#f2b134','#b07a4a','#8a96a3'];

  const defaultLedRules: LedRule[] = [
    { Name: "S1", Source: "LPP", Step: 60 },
    { Name: "S2", Source: "LPP", Step: 30 },
    { Name: "W3", Source: "LPP", Step: 0 },
    { Name: "W4", Source: "LPC", Step: 0 }
  ];

  const defaultForm: Form = {
    lpcValue: 4200, lpcDuration: 600, lppValue: "", lppDuration: 600, fsLpc: "", fsLpp: "", fsDuration: 7200
  };

  function loadSettings(): Settings {
    try {
      return JSON.parse( localStorage.getItem( settingsKey ) ?? "{}" ) as Settings;
    }
    catch {
      return {};
    }
  }

  // installed PV power used to be stored per device; take the first one over
  function initialPvWp( settings: Settings ): number | string {
    if ( undefined !== settings.pvWp )
      return settings.pvWp;
    var perDevice = Object.values( settings.installedPv ?? {} ).find( v => 0 < Number( v ) );
    return perDevice ?? "";
  }

  const pageList = [
    { id: 'control',    label: 'Control' },
    { id: 'connection', label: 'Connection' },
    { id: 'settings',   label: 'Settings' }
  ];

  @Component({
    components: {
      QrcodeVue
    }
  })
  export class ControlBoxPanel extends Vue {
    public readonly pages = pageList;
    public readonly colours = colourList;
    public readonly feedInSteps = [0, 30, 60, 100];
    // §14a EnWG minimum power a controllable consumer must still be allowed
    public readonly consumptionLimitValue = 4200;

    public page = pageList.some( p => '#' + p.id == window.location.hash ) ? window.location.hash.substring( 1 ) : 'control';
    public qrcode = "";

    public remoteServices: RemoteService[] = [];
    public devices: DeviceStatus[] = [];
    public useCaseInfos: UseCaseInfos = {};
    public remoteEntities: EntityInfo[] = [];
    public selectedSki = "";
    public selectedActor: UseCaseInfo | undefined = undefined;
    public selectedEntity: EntityInfo | undefined = undefined;

    public reported: {[key:string]:{[key:string]:ReportedLimit}} = {};
    public failsafe: {[key:string]:{[key:string]:FailsafeValues}} = {};
    public nominalMax: {[key:string]:{[key:string]:number}} = {};
    public monitorings: {[key:string]:{[key:string]:Monitorings}} = {};
    public messages: Notice[] = [];

    private settings = loadSettings();
    public target: string = this.settings.target ?? "all";
    public pvWp: number | string = initialPvWp( this.settings );
    public ledRules: LedRule[] = this.settings.ledRules ?? defaultLedRules.map( r => ({ ...r }) );
    public form: Form = { ...defaultForm, ...( this.settings.form ?? {} ) };
    public colour = { hl: this.settings.colours?.hl ?? colourList[0], sel: this.settings.colours?.sel ?? colourList[0] };

    private socket: WebSocket | undefined;

    mounted() {
      this.setColour( this.colour.hl, 'hl' );
      this.setColour( this.colour.sel, 'sel' );
      window.addEventListener( 'hashchange', () => this.showPage( window.location.hash.substring( 1 ) ) );

      this.socket = new WebSocket( "ws://" + window.location.hostname + ":7080/ws" );

      this.socket.onopen = () => {
        this.sendNotification( MessageType.GetEntityInfos );
        this.sendNotification( MessageType.GetUseCaseInfos );
      };
      this.socket.onclose = event => {
        console.log( "Socket closed: ", event );
        this.socket = undefined;
        this.qrcode = "";
      };
      this.socket.onerror = error => {
        console.log( "Socket error: ", error );
      };
      this.socket.onmessage = event => this.receive( JSON.parse( event.data ) as Message );
    }

    private receive( message: Message ) {
      const ski = message.SKI;
      const uc = message.UseCase ?? "";

      switch ( message.Type ) {
        case MessageType.QRCode:
          this.qrcode = message.Text ?? "";
          break;
        case MessageType.ServiceListChanged:
          this.sendNotification( MessageType.GetServiceList );
          this.sendNotification( MessageType.DeviceStatusList );
          break;
        case MessageType.SelectService:
          if ( "" == this.selectedSki )
            this.selectedSki = message.Text ?? "";
          break;
        case MessageType.GetServiceList:
          this.remoteServices = message.ServiceList ?? [];
          break;
        case MessageType.DeviceStatusList:
          this.devices = message.Devices ?? [];
          break;
        case MessageType.GetEntityInfos:
          this.remoteEntities = message.EntityInfos ?? [];
          break;
        case MessageType.GetUseCaseInfos:
          this.useCaseInfos = { ...this.useCaseInfos, ...( message.UseCaseInfos ?? {} ) };
          break;
        case MessageType.GetConsumptionLimit:
        case MessageType.GetProductionLimit:
          this.slot( this.reported, ski )[uc] = {
            IsActive: message.Limit?.IsActive ?? false,
            Value:    message.Unlimited ? null : message.Limit?.Value ?? 0,
            Duration: message.Limit?.Duration ?? 0
          };
          break;
        case MessageType.GetConsumptionFailsafeValue:
        case MessageType.GetProductionFailsafeValue:
          this.fsSlot( ski, uc ).Value = message.Unlimited ? null : message.Value ?? 0;
          break;
        case MessageType.GetConsumptionFailsafeDuration:
        case MessageType.GetProductionFailsafeDuration:
          this.fsSlot( ski, uc ).Duration = message.Value ?? 0;
          break;
        case MessageType.GetConsumptionNominalMax:
        case MessageType.GetProductionNominalMax:
          this.slot( this.nominalMax, ski )[uc] = message.Value ?? 0;
          break;
        case MessageType.GetPowerLimitationFactor:
          this.monSlot( ski, uc ).PowerLimitationFactor = message.Value;
          break;
        case MessageType.GetPower:
          this.monSlot( ski, uc ).Power = message.Value;
          break;
        case MessageType.GetPowerPerPhase:
          this.monSlot( ski, uc ).PowerPerPhase = message.Values;
          break;
        case MessageType.GetEnergyFeedIn:
          this.monSlot( ski, uc ).EnergyFeedIn = message.Value;
          break;
        case MessageType.GetEnergyConsumed:
          this.monSlot( ski, uc ).EnergyConsumed = message.Value;
          break;
        case MessageType.GetCurrentPerPhase:
          this.monSlot( ski, uc ).CurrentPerPhase = message.Values;
          break;
        case MessageType.GetVoltagePerPhase:
          this.monSlot( ski, uc ).VoltagePerPhase = message.Values;
          break;
        case MessageType.GetFrequency:
          this.monSlot( ski, uc ).Frequency = message.Value;
          break;
        case MessageType.Notice:
          this.addMessage( ski, message.Text ?? "", message.Error ?? false );
          break;
      }
    }

    private slot<T>( map: {[key:string]:{[key:string]:T}}, ski: string ): {[key:string]:T} {
      if ( ! map[ski] )
        map[ski] = {};
      return map[ski];
    }

    private fsSlot( ski: string, uc: string ): FailsafeValues {
      const s = this.slot( this.failsafe, ski );
      if ( ! s[uc] )
        s[uc] = {};
      return s[uc];
    }

    private monSlot( ski: string, uc: string ): Monitorings {
      const s = this.slot( this.monitorings, ski );
      if ( ! s[uc] )
        s[uc] = {};
      return s[uc];
    }

    private addMessage( ski: string, text: string, isError: boolean ) {
      this.messages.unshift( {
        Time:   new Date().toLocaleTimeString( 'en-GB' ),
        Device: ( '' == ski || 'all' == ski ) ? 'all' : this.deviceName( ski ),
        Text:   text,
        Error:  isError
      } );
      this.messages.splice( 50 );
    }

    // --- pages and appearance ---------------------------------------------------------------

    public showPage( id: string ) {
      if ( ! pageList.some( p => p.id == id ) )
        id = 'control';
      this.page = id;
      if ( window.location.hash != '#' + id )
        history.replaceState( null, '', '#' + id );
    }

    public setColour( c: string, kind: 'hl' | 'sel' ) {
      if ( ! colourList.includes( c ) )
        c = colourList[0];
      this.colour[kind] = c;
      document.documentElement.style.setProperty( '--' + kind, c );
      if ( 'hl' == kind ) {
        const r = parseInt( c.substring( 1, 3 ), 16 ), g = parseInt( c.substring( 3, 5 ), 16 ), b = parseInt( c.substring( 5, 7 ), 16 );
        document.documentElement.style.setProperty( '--hl-txt', ( 0.299 * r + 0.587 * g + 0.114 * b ) > 150 ? '#111' : '#fff' );
      }
      this.saveSettings();
    }

    public saveSettings() {
      try {
        localStorage.setItem( settingsKey, JSON.stringify( {
          pvWp:     this.pvWp,
          ledRules: this.ledRules,
          target:   this.target,
          form:     this.form,
          colours:  this.colour
        } as Settings ) );
      }
      catch {
        // settings are a convenience only
      }
    }

    // --- devices and targets ----------------------------------------------------------------

    public get targets(): DeviceStatus[] {
      if ( 'all' == this.target )
        return this.devices;
      return this.devices.filter( d => d.SKI == this.target );
    }

    public get targetName(): string {
      return 'all' == this.target ? 'all connected devices' : this.deviceName( this.target );
    }

    public supports( d: DeviceStatus, uc: string ): boolean {
      return ( d.UseCases ?? [] ).includes( uc );
    }

    public targetsWith( uc: string ): DeviceStatus[] {
      return this.targets.filter( d => this.supports( d, uc ) );
    }

    public isConnected( ski: string ): boolean {
      return this.devices.some( d => d.SKI == ski && d.Connected );
    }

    public deviceName( ski: string ): string {
      const d = this.devices.find( dd => dd.SKI == ski );
      const rs = this.remoteServices.find( r => r.ski == ski );
      const name = [d?.Brand || rs?.brand, d?.Model || rs?.model].filter( x => !! x ).join( ' ' );
      return name || ( ski.substring( 0, 8 ) + '…' );
    }

    public serviceLabel( rs: RemoteService ): string {
      return rs.brand + " " + rs.model + ( "" < rs.serial ? ", SN-" + rs.serial : "" ) + ( this.isConnected( rs.ski ) ? " · connected" : "" );
    }

    public get selectedService(): RemoteService | undefined {
      return this.remoteServices.find( rs => rs.ski == this.selectedSki );
    }

    public isHems( rs: RemoteService ): boolean {
      return ( rs.type ?? "" ).toLowerCase().replace( /\s+/g, '' ).includes( 'energymanag' );
    }

    // Selecting a device that is not connected yet makes ControlBox connect to it.
    public serviceSelected() {
      this.selectedActor = undefined;
      this.selectedEntity = undefined;
      const rs = this.selectedService;
      if ( rs && ! this.isConnected( rs.ski ) && this.isHems( rs ) &&
           ! confirm( 'Attention HEMS: ' + rs.brand + ' ' + rs.model + ' connects on its own.\nDo you really want to connect to it?' ) ) {
        this.addMessage( rs.ski, 'Connecting cancelled — the energy manager connects on its own.', false );
        return;
      }
      this.sendNotification( MessageType.SelectService, this.selectedSki );
    }

    public get actorList(): UseCaseInfo[] {
      return this.useCaseInfos[this.selectedSki] ?? [];
    }

    public get selectedEntities(): EntityInfo[] {
      return this.remoteEntities.filter( re => re.SKI == this.selectedSki );
    }

    // --- sending ----------------------------------------------------------------------------

    private send( type: MessageType, fields: Partial<Message> ) {
      if ( ! this.socket )
        return;
      this.socket.send( JSON.stringify( { SKI: this.target, Type: type, ...fields } ) );
    }

    private sendNotification( type: MessageType, param: string = "" ) {
      if ( ! this.socket )
        return;
      this.socket.send( JSON.stringify( { SKI: this.selectedSki, Type: type, Text: param } ) );
    }

    public isEmpty( v: number | string | null | undefined ): boolean {
      return null === v || undefined === v || "" === v;
    }

    private num( v: number | string ): number {
      return Number( v ) || 0;
    }

    public sendConsumptionLimit( active: boolean ) {
      const value = this.isEmpty( this.form.lpcValue ) ? this.consumptionLimitValue : this.num( this.form.lpcValue );
      this.send( MessageType.SetConsumptionLimit, { Limit: { IsActive: active, Value: value, Duration: active ? this.num( this.form.lpcDuration ) : 0 } } );
      this.addMessage( this.target, active ? 'Consumption limit ' + this.formatted( value ) + ' W sent' : 'Consumption limit released', false );
    }

    // feed-in limits are sent as negative values, as the devices expect and report them
    public sendProductionLimit( active: boolean ) {
      const amount = Math.abs( this.num( this.form.lppValue ) );
      this.send( MessageType.SetProductionLimit, { Limit: { IsActive: active, Value: -amount, Duration: active ? this.num( this.form.lppDuration ) : 0 } } );
      this.addMessage( this.target, active ? 'Feed-in limit ' + this.formatted( amount ) + ' W sent' : 'Feed-in limit released', false );
    }

    // Quick buttons fill the form and send at once.
    public setConsumption( active: boolean ) {
      this.form.lpcValue = this.consumptionLimitValue;
      this.saveSettings();
      this.sendConsumptionLimit( active );
    }

    public setFeedIn( pct: number ) {
      const wp = this.num( this.pvWp );
      if ( pct < 100 && ! wp )
        return;
      if ( pct < 100 )
        this.form.lppValue = Math.round( wp * pct / 100 );
      this.saveSettings();
      // 100 % means no limitation: release instead of limiting to full power
      this.sendProductionLimit( pct < 100 );
    }

    public sendFailsafeValue( uc: 'LPC' | 'LPP' ) {
      const amount = Math.abs( this.num( 'LPC' == uc ? this.form.fsLpc : this.form.fsLpp ) );
      this.send( 'LPC' == uc ? MessageType.SetConsumptionFailsafeValue : MessageType.SetProductionFailsafeValue,
                 { Value: 'LPC' == uc ? amount : -amount } );
    }

    // one minimum duration for consumption and feed-in, sent to the use cases the targets support
    public sendFailsafeDuration() {
      const seconds = this.num( this.form.fsDuration );
      if ( this.targetsWith( 'LPC' ).length )
        this.send( MessageType.SetConsumptionFailsafeDuration, { Value: seconds } );
      if ( this.targetsWith( 'LPP' ).length )
        this.send( MessageType.SetProductionFailsafeDuration, { Value: seconds } );
    }

    // --- state ------------------------------------------------------------------------------

    // true if every target supporting the use case confirmed a limit matching test
    public allConfirm( uc: string, test: (l: ReportedLimit) => boolean ): boolean {
      const ts = this.targetsWith( uc );
      return 0 < ts.length && ts.every( d => {
        const l = this.reported[d.SKI]?.[uc];
        return !! l && test( l );
      } );
    }

    public get feedInStepsHint(): string {
      return this.num( this.pvWp ) ? "" : "Enter the installed PV power first (Settings → Installation)";
    }

    public feedInHint( pct: number ): string {
      if ( 100 <= pct )
        return "Release the feed-in limit";
      if ( ! this.num( this.pvWp ) )
        return this.feedInStepsHint;
      return "Limit feed-in to " + this.formatted( Math.round( this.num( this.pvWp ) * pct / 100 ) ) + " W";
    }

    // allowed feed-in in % of installed PV power, 100 if not limited, undefined if unknown
    public feedInPct( l: ReportedLimit | undefined ): number | undefined {
      if ( ! l )
        return undefined;
      if ( ! l.IsActive || null === l.Value )
        return 100;
      const wp = this.num( this.pvWp );
      if ( ! wp )
        return undefined;
      return Math.min( 100, 100 * Math.abs( l.Value ) / wp );
    }

    public isFeedInStep( l: ReportedLimit, pct: number ): boolean {
      const p = this.feedInPct( l );
      return undefined !== p && Math.abs( p - pct ) < 0.5;
    }

    public limitText( ski: string, uc: string ): string {
      const l = this.reported[ski]?.[uc];
      if ( ! l )
        return '—';
      if ( ! l.IsActive || null === l.Value )
        return 'not limited';
      var text = 'limited to ' + this.formatted( Math.abs( l.Value ) ) + ' W';
      if ( 'LPP' == uc ) {
        const p = this.feedInPct( l );
        if ( undefined !== p )
          text += ' (' + Math.round( p ) + ' %)';
      }
      if ( 0 < l.Duration )
        text += ', ' + this.durationText( l.Duration );
      return text;
    }

    public ledState( rule: LedRule, ski: string ): 'on' | 'off' | 'unknown' {
      const d = this.devices.find( dd => dd.SKI == ski );
      if ( 'off' == rule.Source || ( d && ! this.supports( d, rule.Source ) ) )
        return 'off';
      if ( 'LPC' == rule.Source ) {
        const l = this.reported[ski]?.['LPC'];
        return l ? ( l.IsActive ? 'on' : 'off' ) : 'unknown';
      }
      if ( 'LPP' == rule.Source ) {
        const p = this.feedInPct( this.reported[ski]?.['LPP'] );
        if ( undefined === p )
          return 'unknown';
        if ( 100 <= p )
          return 'off';
        // the output for the highest step not exceeding the allowed feed-in
        const steps = this.ledRules.filter( r => 'LPP' == r.Source ).map( r => r.Step );
        const allowed = steps.filter( step => step <= p + 0.5 );
        const step = allowed.length ? Math.max( ...allowed ) : Math.min( ...steps );
        return rule.Step == step ? 'on' : 'off';
      }
      return 'off';
    }

    public ledHint( rule: LedRule ): string {
      switch ( rule.Source ) {
        case 'LPC': return rule.Name + ": consumption limit active";
        case 'LPP': return rule.Name + ": feed-in limited to " + rule.Step + " %";
        default:    return rule.Name + ": not used";
      }
    }

    // --- formatting -------------------------------------------------------------------------

    public wattText( v: number | null | undefined ): string {
      if ( undefined === v )
        return '—';
      return null === v ? 'unlimited' : this.formatted( Math.abs( v ) ) + ' W';
    }

    public durationText( s: number | undefined ): string {
      if ( undefined === s )
        return '—';
      if ( 0 == s )
        return 'unlimited';
      return this.formatted( s ) + ' s' + ( 0 == s % 3600 ? ' (' + s / 3600 + ' h)' : 0 == s % 60 ? ' (' + s / 60 + ' min)' : '' );
    }

    public nominalText( ski: string ): string {
      const n = this.nominalMax[ski];
      if ( ! n )
        return '—';
      return Object.keys( n ).map( uc => uc + ' ' + this.formatted( n[uc] ) + ' W' ).join( ' · ' );
    }

    public phases( values: number[] | undefined ): string {
      return values?.length ? values.map( v => this.formatted( v ) ).join( ' / ' ) : '—';
    }

    public countText( n: number, singular: string, plural: string = singular + 's' ): string {
      return n + ' ' + ( 1 == n ? singular : plural );
    }

    public readableSKI( ski: string ): string {
      if ( 40 < ski.length )
        return ski;
      var parts: string[] = [];
      for ( var i = 0; i < 10; i++ )
        parts[i] = ski.substring( 4*i, 4*i +4 );
      return parts.join( ' ' );
    }

    public formatted( val: number ): string {
      return new Intl.NumberFormat( 'en-US' ).format( val );
    }
  }

  export default toNative( ControlBoxPanel )
</script>

<style scoped>
  h1 {
    font-size: 20px;
    margin: 0;
    padding: 14px 16px;
    background: var(--panel);
    border-bottom: 1px solid var(--line);
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
  }
  h1 > span:first-child {
    color: var(--hl);
  }
  h2 {
    font-size: 15px;
    margin: 0 0 12px;
    color: var(--hl);
    letter-spacing: .02em;
  }
  h3 {
    font-size: 12px;
    margin: 0 0 10px;
    color: var(--muted);
    text-transform: uppercase;
    letter-spacing: .06em;
  }
  h3.spaced {
    margin-top: 14px;
  }
  .wrap {
    max-width: 1080px;
    margin: 0 auto;
    padding: 14px;
  }
  .card {
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: 10px;
    padding: 14px 16px;
    margin: 0 0 14px;
  }
  .cardhead {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 10px;
    margin: 0 0 12px;
  }
  .cardhead h2 {
    margin: 0;
  }

  /* pages in the title bar */
  .pages {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }
  .pages button {
    background: var(--panel2);
    border: 1px solid var(--line);
    color: var(--muted);
    font-size: 14px;
    padding: 7px 16px;
  }
  .pages button.active {
    background: var(--hl);
    border-color: var(--hl);
    color: var(--hl-txt);
  }

  button.grey {
    background: #5a6370;
    color: #fff;
  }
  button.sec {
    background: var(--panel2);
    border: 1px solid var(--line);
    color: var(--txt);
  }
  button.icon {
    padding: 6px 9px;
    line-height: 0;
  }

  /* consumption and feed-in side by side, below each other when narrow */
  .duo {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 14px;
    margin: 0 0 14px;
  }
  .duo .card {
    margin: 0;
  }
  @media (max-width: 760px) {
    .duo, .grid2 {
      grid-template-columns: 1fr;
    }
  }
  .grid2 {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 22px;
  }

  .devbar {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
  }
  .devbar select {
    flex: 1 1 300px;
    width: auto;
    min-width: 0;
  }

  .row2 {
    display: grid;
    grid-template-columns: 110px 1fr;
    gap: 8px;
    align-items: center;
    margin: 7px 0;
  }
  .row2 label, .row label {
    color: var(--muted);
  }
  .row {
    display: grid;
    grid-template-columns: 180px 1fr auto;
    gap: 8px 10px;
    align-items: center;
    margin: 7px 0;
  }
  .row input {
    min-width: 130px;
  }
  @media (max-width: 640px) {
    .row {
      grid-template-columns: 1fr;
    }
  }

  /* quick buttons share the whole line, action buttons are centred */
  .quick {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
    align-items: center;
    margin: 4px 0 12px;
  }
  .quick.fill button {
    flex: 1 1 0;
    min-width: 0;
  }
  .quick.center {
    justify-content: center;
  }
  .quick button.sel {
    outline: 2px solid var(--sel);
    outline-offset: 2px;
  }

  .hint {
    color: var(--muted);
    font-size: 12px;
    margin: 10px 0 0;
  }
  .hint.first, .hint.inline {
    margin: 0;
  }
  .hint.below {
    margin: -4px 0 14px;
  }

  .pill {
    display: inline-block;
    padding: 3px 10px;
    border-radius: 20px;
    font-size: 12px;
    background: var(--panel2);
    border: 1px solid var(--line);
    white-space: nowrap;
  }
  .pill.ok {
    color: var(--ok);
    border-color: var(--ok);
  }
  .pill.bad {
    color: var(--bad);
    border-color: var(--bad);
  }

  .tab {
    width: 100%;
    border-collapse: collapse;
    margin: 4px 0 10px;
    font-size: 13px;
  }
  .tab th, .tab td {
    text-align: left;
    padding: 5px 8px;
    border-bottom: 1px solid var(--line);
  }
  .tab th {
    color: var(--muted);
    font-weight: 600;
  }
  .msgtab th:nth-child(-n+2), .msgtab td:nth-child(-n+2) {
    width: 1%;
    white-space: nowrap;
  }
  .msgtab th:nth-child(3), .msgtab td:nth-child(3) {
    width: 22%;
  }
  .tag {
    display: inline-block;
    padding: 1px 8px;
    border-radius: 10px;
    font-size: 11px;
    font-weight: 700;
    border: 1px solid;
  }
  .tag.error {
    color: var(--bad);
  }
  .tag.info {
    color: var(--warn);
  }

  .mono {
    font-family: ui-monospace, Consolas, monospace;
    font-size: 12px;
    word-break: break-all;
  }

  /* output LEDs */
  .ledi {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    font-weight: 600;
    margin-right: 12px;
  }
  .led {
    display: inline-block;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    border: 1px solid var(--line);
    background: #2a2f36;
  }
  .led.on {
    background: var(--ok);
    border-color: var(--ok);
    box-shadow: 0 0 8px var(--ok);
  }
  .led.unknown {
    background: transparent;
    border-style: dashed;
    border-color: var(--muted);
  }
  .ledcfg {
    display: grid;
    grid-template-columns: 40px max-content max-content;
    gap: 6px 10px;
    align-items: center;
    margin-top: 8px;
  }
  .ledcfg input {
    width: 70px;
  }

  .swatches {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin: 6px 0;
  }
  .swatch {
    width: 32px;
    height: 32px;
    padding: 0;
    border-radius: 7px;
    border: 2px solid var(--line);
  }
  .swatch.active {
    border-color: var(--txt);
    box-shadow: 0 0 0 2px var(--bg) inset;
  }

  .identity {
    display: grid;
    grid-template-columns: 130px 1fr;
    gap: 16px;
    align-items: center;
  }
  .qrcode {
    width: 130px;
    height: 130px;
    background: #fff;
    padding: 4px;
    border-radius: 6px;
  }
  .ident {
    display: grid;
    grid-template-columns: 110px 1fr;
    gap: 4px 10px;
    margin-top: 8px;
  }
  .ident b, .info b {
    color: var(--muted);
    font-weight: 600;
  }
  .info {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 5px 12px;
    margin-bottom: 14px;
  }
  .info span {
    text-align: right;
    font-family: ui-monospace, Consolas, monospace;
    font-size: 13px;
  }
</style>
