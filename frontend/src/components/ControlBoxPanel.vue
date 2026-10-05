<template>
  <h1>Control Box Simulator</h1>
  <div class="header-frame">
    <div class="header" v-if="'' < qrcode">
      <label class="qrcode-text">{{ qrcode }}</label>
      <qrcode-vue  class="qrcode" :value="qrcode" :size="130" level="H" render-as="svg" />
    </div>
  </div>
  <div v-if="'' == qrcode">
    <h3>not running</h3>
  </div>
  <div v-else>
    <div v-if="0 < remoteServices?.length" class="devices">
      <label class="device-select-label">Remote Device:</label>
      <VueSelect v-model="selectedSki" :options="optionServices"
        placeholder="Select a device" @option-selected="serviceSelected">
      </VueSelect>
    </div>
    <div v-else>
      <h3>No devices found</h3>
    </div>

    <div v-if="'' < selectedSki" class="devices">
      <label class="device-select-label">SKI:</label>
      <label class="device-select-label">{{ readableSKI( selectedSki ) }}</label>
      <label class="device-select-label">ID:</label>
      <label class="device-select-label">{{ deviceId }}</label>
      <label class="device-select-label">Device Brand:</label>
      <label class="device-select-label">{{ deviceBrand }}</label>
      <label class="device-select-label">Device Type:</label>
      <label class="device-select-label">{{ deviceType }}</label>
      <label class="device-select-label">Device Model:</label>
      <label class="device-select-label">{{ deviceModel }}</label>
      <label class="device-select-label">Serial Number:</label>
      <label class="device-select-label">{{ deviceSerial }}</label>
    </div>

    <div v-if="'' < selectedSki && !! remoteEntities" class="devices">
      <label class="device-select-label">Actors:</label>
      <VueSelect v-model="selectedActor" :options="optionActors"
        v-bind:placeholder="optionActors.length + (optionActors.length == 1 ? ' actor' : ' actors')">
      </VueSelect>
      <label class="device-select-label">Usecases:</label>
      <VueSelect :options="optionUsecases"
        v-bind:placeholder="! selectedActor ? '' : (optionUsecases.length + (optionUsecases.length == 1 ? ' usecase' : ' usecases'))">
      </VueSelect>
      <label class="device-select-label">Entities:</label>
      <VueSelect v-model="selectedEntity" :options="optionEntities"
        v-bind:placeholder="optionEntities.length + (optionEntities.length == 1 ? ' entity' : ' entities')">
      </VueSelect>
      <label class="device-select-label">Features:</label>
      <VueSelect :options="optionFeatures"
        v-bind:placeholder="! selectedEntity ? '' : (optionFeatures.length + (optionFeatures.length == 1 ? ' feature' : ' features'))">
      </VueSelect>
    </div>
    <div v-if="'' < selectedSki && (existsUC('limitationOfPowerProduction') || existsUC('limitationOfPowerConsumption'))" class="outputs">
      <h3>ERC Outputs</h3>
      <p v-if="! hasInstalledPv && ledRules.some( r => 'LPP' == r.Source )" class="hint">
        Feed-in outputs need the installed PV power (Production Limit panel).
      </p>
      <div class="leds">
        <div v-for="rule in ledRules" :key="rule.Name" class="led-item" :title="ledHint( rule )">
          <span :class="['led', ledState( rule )]"></span>
          <label>{{ rule.Name }}</label>
        </div>
      </div>
      <details class="led-config">
        <summary>Configure outputs</summary>
        <div class="led-config-grid">
          <template v-for="rule in ledRules" :key="rule.Name">
            <label>{{ rule.Name }}</label>
            <select v-model="rule.Source" @change="saveSettings">
              <option value="LPP">Feed-in limited to</option>
              <option value="LPC">Consumption limit active</option>
              <option value="off">Not used</option>
            </select>
            <span v-if="rule.Source == 'LPP'"><input type="number" v-model.number="rule.Step" min="0" max="100" @change="saveSettings" /> %</span>
            <span v-else></span>
          </template>
        </div>
      </details>
    </div>

    <div class="usecases">
      <div v-if="'' < selectedSki && !!selectedLs && !!selectedLs['LPC'] && existsUC('limitationOfPowerConsumption')">
        <h3>Consumption Limit</h3>
        <div class="quick-buttons">
          <button type="button" :class="{ selected: isConsumptionLimited( true ) }"
            :title="'Limit consumption to ' + formatted( consumptionLimitValue ) + ' W'"
            @click="setConsumption( true )">Limit</button>
          <button type="button" :class="{ selected: isConsumptionLimited( false ) }"
            title="Deactivate the consumption limit"
            @click="setConsumption( false )">No Limit</button>
        </div>
        <div class="form-line3">
          <label>Active:</label>
          <input type="checkbox" v-model="selectedLs['LPC'].IsActive" @input="onLPCUserChanged" />

          <label>Dimmed Value [W]:</label>
          <input type="number" v-model="selectedLs['LPC'].Value" placeholder="unlimited" @input="onLPCUserChanged" />
          <button class="three-lines" type="button" @click="setConsumptionLimit"
            :disabled="isUnlimited( selectedLs['LPC'].Value )" :title="unlimitedHint( selectedLs['LPC'].Value )">Set</button>

          <label>Dimmed Duration [s]:</label>
          <input type="number" v-model="selectedLs['LPC'].Duration" @input="onLPCUserChanged" />

          <label>Failsafe Value [W]:</label>
          <input type="number" v-model="selectedLs['LPC'].FSValue" placeholder="unlimited" @input="onLPCUserChanged" />
          <button type="button" @click="setConsumptionFailsafeLimit"
            :disabled="isUnlimited( selectedLs['LPC'].FSValue )" :title="unlimitedHint( selectedLs['LPC'].FSValue )">Set</button>

          <label>Failsafe Duration [s]:</label>
          <input type="number" v-model="selectedLs['LPC'].FSDuration" @input="onLPCUserChanged" />
          <button type="button" @click="setConsumptionFailsafeDuration">Set</button>

          <label>Nominal Maximum [W]:</label>
          <input type="number" v-model="consumptionNominalMax[selectedSki]" />
          <div></div>

          <label>Received Heartbeat:</label>
          <span v-bind:class = "(consumptionHeartbeat)?'pulse heartbeat':'pulse'">&#9673;</span>
          <!-- <button type="button" @click="toggleConsumptionHeartbeat">{{ consumptionHeartbeatEnabled ? 'Stop' : 'Start' }}</button> -->
          <div></div>
        </div>
      </div>

      <div v-if="'' < selectedSki && !!selectedLs && !!selectedLs['LPP'] && existsUC('limitationOfPowerProduction')">
        <h3>Production Limit</h3>
        <div class="quick-buttons">
          <button v-for="pct in feedInSteps" :key="pct" type="button"
            :class="{ selected: isFeedInStep( pct ) }" :disabled="! hasInstalledPv"
            :title="hasInstalledPv ? feedInHint( pct ) : 'Enter the installed PV power first'"
            @click="setFeedIn( pct )">{{ pct }} %</button>
          <span v-if="! hasInstalledPv" class="hint">Enter installed PV power first (below)</span>
        </div>
        <div class="form-line3">
          <label>Active:</label>
          <input type="checkbox" v-model="selectedLs['LPP'].IsActive" @input="onLPPUserChanged" />

          <label>Dimmed Value [W]:</label>
          <input type="number" v-model="selectedLs['LPP'].Value" placeholder="unlimited" @input="onLPPUserChanged" />
          <button class="three-lines" type="button" @click="setProductionLimit"
            :disabled="isUnlimited( selectedLs['LPP'].Value )" :title="unlimitedHint( selectedLs['LPP'].Value )">Set</button>
          
          <label>Dimmed Duration [s]:</label>
          <input type="number" v-model="selectedLs['LPP'].Duration" @input="onLPPUserChanged" />
          
          <label>Failsafe Value [W]:</label>
          <input type="number" v-model="selectedLs['LPP'].FSValue" placeholder="unlimited" @input="onLPPUserChanged" />
          <button type="button" @click="setProductionFailsafeLimit"
            :disabled="isUnlimited( selectedLs['LPP'].FSValue )" :title="unlimitedHint( selectedLs['LPP'].FSValue )">Set</button>
          
          <label>Failsafe Duration [s]:</label>
          <input type="number" v-model="selectedLs['LPP'].FSDuration" @input="onLPPUserChanged" />
          <button type="button" @click="setProductionFailsafeDuration">Set</button>
          
          <label>Nominal Maximum [W]:</label>
          <input type="number" v-model="productionNominalMax[selectedSki]" />
          <div></div>

          <label>Installed PV Power [Wp]:</label>
          <input type="number" v-model.number="installedPv[selectedSki]" placeholder="Wp" @change="saveSettings"
            :class="{ missing: ! hasInstalledPv }" />
          <div></div>

          <label>Received Heartbeat:</label>
          <span v-bind:class = "(productionHeartbeat)?'pulse heartbeat':'pulse'">&#9673;</span>
          <!-- <button type="button" @click="toggleProductionHeartbeat">{{ productionHeartbeatEnabled ? 'Stop' : 'Start' }}</button> -->
          <div></div>
        </div>
      </div>

      <div v-if="'' < selectedSki && !!selectedMs && !!selectedMs['MGCP'] && existsUC('monitoringOfGridConnectionPoint')">
        <h3>Monitoring Grid Connection Point</h3>
        <div class="form-line2">
          <label>Power Limitation Factor:</label>
          <label>{{ selectedMs['MGCP'].PowerLimitationFactor ?? 0 }} %</label>

          <label>Power:</label>
          <label>{{ formatted( selectedMs['MGCP'].Power ?? 0 ) }} W</label>

          <label>Energy FeedIn:</label>
          <label>{{ formatted( selectedMs['MGCP'].EnergyFeedIn ?? 0 ) }} Wh</label>

          <label>Energy Consumed:</label>
          <label>{{ formatted( selectedMs['MGCP'].EnergyConsumed ?? 0 ) }} Wh</label>

          <label>Currents per Phase:</label>
          <label>{{ ! selectedMs['MGCP'].CurrentPerPhase ? '0' : selectedMs['MGCP'].CurrentPerPhase[0] }} A,
                 {{ ! selectedMs['MGCP'].CurrentPerPhase ? '0' : selectedMs['MGCP'].CurrentPerPhase[1] }} A,
                 {{ ! selectedMs['MGCP'].CurrentPerPhase ? '0' : selectedMs['MGCP'].CurrentPerPhase[2] }} A
          </label>

          <label>Voltages per Phase:</label>
          <label>{{ ! selectedMs['MGCP'].VoltagePerPhase ? '0' : selectedMs['MGCP'].VoltagePerPhase[0] }} V,
                 {{ ! selectedMs['MGCP'].VoltagePerPhase ? '0' : selectedMs['MGCP'].VoltagePerPhase[1] }} V,
                 {{ ! selectedMs['MGCP'].VoltagePerPhase ? '0' : selectedMs['MGCP'].VoltagePerPhase[2] }} V
          </label>

          <label>Frequency:</label>
          <label>{{ selectedMs['MGCP'].Frequency ?? 0 }} Hz</label>
        </div>
      </div>

      <div v-if="'' < selectedSki && !!selectedMs && !!selectedMs['MPC'] && existsUC('monitoringOfPowerConsumption')">
        <h3>Monitoring Power Consumption</h3>
        <div class="form-line2">
          <label>Power:</label>
          <label>{{ selectedMs['MPC'].Power ?? 0 }} W</label>

          <label>Power per Phase:</label>
          <label>{{ ! selectedMs['MPC'].PowerPerPhase ? '0' : selectedMs['MPC'].PowerPerPhase[0] }} W,
                 {{ ! selectedMs['MPC'].PowerPerPhase ? '0' : selectedMs['MPC'].PowerPerPhase[1] }} W,
                 {{ ! selectedMs['MPC'].PowerPerPhase ? '0' : selectedMs['MPC'].PowerPerPhase[2] }} W
          </label>

          <label>Energy FeedIn:</label>
          <label>{{ selectedMs['MPC'].EnergyFeedIn ?? 0 }} Wh</label>

          <label>Energy Consumed:</label>
          <label>{{ selectedMs['MPC'].EnergyConsumed ?? 0 }} Wh</label>

          <label>Currents per Phase:</label>
          <label>{{ ! selectedMs['MPC'].CurrentPerPhase ? '0' : selectedMs['MPC'].CurrentPerPhase[0] }} A,
                 {{ ! selectedMs['MPC'].CurrentPerPhase ? '0' : selectedMs['MPC'].CurrentPerPhase[1] }} A,
                 {{ ! selectedMs['MPC'].CurrentPerPhase ? '0' : selectedMs['MPC'].CurrentPerPhase[2] }} A
          </label>

          <label>Voltages per Phase:</label>
          <label>{{ ! selectedMs['MPC'].VoltagePerPhase ? '0' : selectedMs['MPC'].VoltagePerPhase[0] }} V,
                 {{ ! selectedMs['MPC'].VoltagePerPhase ? '0' : selectedMs['MPC'].VoltagePerPhase[1] }} V,
                 {{ ! selectedMs['MPC'].VoltagePerPhase ? '0' : selectedMs['MPC'].VoltagePerPhase[2] }} V
          </label>

          <label>Frequency:</label>
          <label>{{ selectedMs['MPC'].Frequency ?? 0 }} Hz</label>
        </div>
      </div>

    </div>
  </div>
</template>

<script lang="ts">
  import { Component, Vue, toNative } from 'vue-facing-decorator'
  import QrcodeVue from 'qrcode.vue'
  import VueSelect from 'vue3-select-component'

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
	  GetFrequency                   = 36
}

  interface Limits {
    IsActive:   boolean,
	  Value:      number | null,   // null: no limit (device reported +Inf)
	  Duration:   number,
	  FSValue:    number | null,   // null: no limit (device reported +Inf)
	  FSDuration: number
  }

  // An ERC output LED: lit for a feed-in step (LPP, in % of installed PV
  // power) or while a consumption limit (LPC) is active.
  interface LedRule {
    Name:   string,
    Source: 'LPP' | 'LPC' | 'off',
    Step:   number
  }

  interface ReportedLimit {
    IsActive: boolean,
    Value:    number | null
  }

  interface Settings {
    installedPv?: {[key:string]:number | string},
    ledRules?:    LedRule[]
  }

  const settingsKey = "controlbox.settings";

  const defaultLedRules: LedRule[] = [
    { Name: "S1", Source: "LPP", Step: 60 },
    { Name: "S2", Source: "LPP", Step: 30 },
    { Name: "W3", Source: "LPP", Step: 0 },
    { Name: "W4", Source: "LPC", Step: 0 }
  ];

  function loadSettings(): Settings {
    try {
      return JSON.parse( localStorage.getItem( settingsKey ) ?? "{}" ) as Settings;
    }
    catch {
      return {};
    }
  }

  interface Monitorings {
    PowerLimitationFactor: number,
    Power: number,
    PowerPerPhase: number[],
    EnergyFeedIn: number,
    EnergyConsumed: number,
    CurrentPerPhase: number[],
    VoltagePerPhase: number[],
    Frequency: number
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
    Limit?:        Limits,
    Value?:        number,
    Unlimited?:    boolean,
    Values?:       number[],
    ServiceList?:  RemoteService[],
    EntityInfos?:  EntityInfo[],
    UseCaseInfos?: UseCaseInfos
    UseCase?:      string
  }

  type UCLimits       = {[key:string]:Limits};
  type LimitData      = {[key:string]:UCLimits};
  type UCMonitorings  = {[key:string]:Monitorings};
  type MonitoringData = {[key:string]:UCMonitorings};

  @Component({
    components: {
      QrcodeVue,
      VueSelect
    }
  })
  export class ControlBoxPanel extends Vue {
    public qrcode = "";

    public limits: LimitData = {};
    public monitorings: MonitoringData = {};

    public remoteServices: RemoteService[] = [];
    public useCaseInfos: UseCaseInfos = {};
    public selectedActor: UseCaseInfo | undefined = undefined
    public remoteEntities: EntityInfo[] = [];
    public selectedSki = "";
    public selectedEntity: EntityInfo | undefined = undefined;

    private lpcUserChanged = false;
    private lppUserChanged = false;

    public readonly feedInSteps = [0, 30, 60, 100];
    public installedPv: {[key:string]:number | string} = loadSettings().installedPv ?? {};
    public ledRules: LedRule[] = loadSettings().ledRules ?? defaultLedRules.map( r => ({ ...r }) );

    // limits as confirmed by the device, independent of unsent form input
    private reportedLimits: {[key:string]:{[key:string]:ReportedLimit}} = {};

    public existsUC( uc: string ): boolean {
      var exists = false;
      
      this.useCaseInfos[this.selectedSki]?.forEach( uci => {
        exists ||= uci.Names.includes( uc );
      } );

      return exists;
    }

    public get selectedLs() {
      return this.limits[this.selectedSki];
    }

    public get selectedMs() {
      return this.monitorings[this.selectedSki];
    }

    public get optionServices() {
      var options:any[] = [];
      this.remoteServices.forEach(item => { options.push({
          label: item.brand + " " + item.model + ("" < item.serial ? (", SN-" + item.serial) : ""),
          value: item.ski
        });        
      });
      return options;
    }

    public get optionActors() {
      var options:any[] = [];

      if ( "" < this.selectedSki && !! this.useCaseInfos[this.selectedSki]) {
        this.useCaseInfos[this.selectedSki].forEach(item => { options.push( {
            label: item.Actor,
            value: item
          });        
        });
      }
      return options;
    }

    public get optionUsecases() {
      var options:any[] = [];

      if ( "" < this.selectedSki && !! this.selectedActor ) {
        this.selectedActor.Names.forEach( item => { options.push( {
            label: item,
            value: item
          });        
        });
      }
      return options;
    }

    public get selectedEntities(): EntityInfo[] {
      if ( "" < this.selectedSki && !! this.remoteEntities ) {
        return this.remoteEntities.filter( re => re.SKI == this.selectedSki );
      }
      else {
        return [];
      }
    }

    public get optionEntities() {
      var options:any[] = [];

      if ( "" < this.selectedSki && !! this.selectedEntities ) {
        this.selectedEntities.forEach(item => { options.push({
            label: item.Name,
            value: item
          });        
        });
      }
      return options;
    }

    public get optionFeatures() {
      var options:any[] = [];

      if ( "" < this.selectedSki && !! this.selectedEntity ) {
        this.selectedEntity.Features.forEach(item => { options.push({
            label: item,
            value: item
          });        
        });
      }
      return options;
    }

    public get deviceId() {
      if ( "" < this.selectedSki ) {
        var remoteService = this.remoteServices.find( rs => rs.ski == this.selectedSki );
        return remoteService?.shipID ?? "";
      }
      else {
        return "";
      }
    }

    public get deviceBrand() {
      if ( "" < this.selectedSki ) {
        var remoteService = this.remoteServices.find( rs => rs.ski == this.selectedSki );
        return remoteService?.brand ?? "";
      }
      else {
        return "";
      }
    }

    public get deviceType() {
      if ( "" < this.selectedSki ) {
        var remoteService = this.remoteServices.find( rs => rs.ski == this.selectedSki );
        return remoteService?.type ?? "";
      }
      else {
        return "";
      }
    }

    public get deviceModel() {
      if ( "" < this.selectedSki ) {
        var remoteService = this.remoteServices.find( rs => rs.ski == this.selectedSki );
        return remoteService?.model ?? "";
      }
      else {
        return "";
      }
    }

    public get deviceSerial() {
      if ( "" < this.selectedSki ) {
        var remoteService = this.remoteServices.find( rs => rs.ski == this.selectedSki );
        return remoteService?.serial ?? "";
      }
      else {
        return "";
      }
    }

    public consumptionNominalMax: {[key: string]: number} = {};
    public productionNominalMax:  {[key: string]: number} = {};

    public consumptionHeartbeat:        boolean = false;
    public consumptionHeartbeatEnabled: boolean = true;
    public productionHeartbeat:         boolean = false;
    public productionHeartbeatEnabled:  boolean = true;

    private socket: WebSocket | undefined;
  
    mounted() {
      this.socket = new WebSocket( "ws://" + window.location.hostname + ":7080/ws" );
      console.log( "Attempting Connection..." );

      this.socket.onopen = () => {
          console.log( "Successfully Connected" );
          this.sendNotification( MessageType.GetEntityInfos );
      };
      
      this.socket.onclose = event => {
          console.log( "Socket Closed Connection: ", event );
          this.sendText( "Client Closed!" );
          this.socket = undefined;
      };

      this.socket.onerror = error => {
          console.log( "Socket Error: ", error );
      };

      this.socket.onmessage = event => {
        //console.log( "Socket message: ", event.data );
        var message: Message = JSON.parse( event.data );
        switch ( message.Type ) {
          case MessageType.QRCode: {
            this.qrcode = message.Text as string;
            console.log( "SHIPID: ", this.qrcode );
            break;
          }
          case MessageType.ServiceListChanged: {
            this.sendNotification( MessageType.GetServiceList );
            break;
          }
          case MessageType.SelectService: {
            if ( this.selectedSki == "" )
              this.selectedSki = message.Text ?? "";
            break;
          }
          case MessageType.GetServiceList: {
            this.remoteServices = message.ServiceList!;
            break;
          }
          case MessageType.GetEntityInfos: {
            this.syncEntities( message.EntityInfos );
            break;
          }
          case MessageType.GetUseCaseInfos: {
            this.syncUseCases( message.UseCaseInfos );
            break;
          }
          case MessageType.GetConsumptionLimit: {
            this.setReportedLimit( message );
            if ( ! this.lpcUserChanged ) {
              this.updateDeviceData( message.UseCase! );
              this.limits[message.SKI][message.UseCase!].IsActive = message.Limit?.IsActive ?? false;
              this.limits[message.SKI][message.UseCase!].Value    = message.Unlimited ? null : message.Limit?.Value ?? 0;
              this.limits[message.SKI][message.UseCase!].Duration = message.Limit?.Duration ?? 0;
            }
            break;
          }
          case MessageType.GetProductionLimit: {
            this.setReportedLimit( message );
            if ( ! this.lppUserChanged ) {
              this.updateDeviceData( message.UseCase! );
              this.limits[message.SKI][message.UseCase!].IsActive = message.Limit?.IsActive ?? false;
              this.limits[message.SKI][message.UseCase!].Value    = message.Unlimited ? null : message.Limit?.Value ?? 0;
              this.limits[message.SKI][message.UseCase!].Duration = message.Limit?.Duration ?? 0;
            }
            break;
          }
          case MessageType.GetConsumptionFailsafeValue: {
            if ( ! this.lpcUserChanged ) {
              this.updateDeviceData( message.UseCase! );
              this.limits[message.SKI][message.UseCase!].FSValue = message.Unlimited ? null : message.Value ?? 0;
            }
            break;
          }
          case MessageType.GetProductionFailsafeValue: {
            if ( ! this.lppUserChanged ) {
              this.updateDeviceData( message.UseCase! );
              this.limits[message.SKI][message.UseCase!].FSValue = message.Unlimited ? null : message.Value ?? 0;
            }
            break;
          }
          case MessageType.GetConsumptionFailsafeDuration: {
            if ( ! this.lpcUserChanged ) {
              this.updateDeviceData( message.UseCase! );
              this.limits[message.SKI][message.UseCase!].FSDuration = message.Value ?? 0;
            }
            break;
          }
          case MessageType.GetProductionFailsafeDuration: {
            if ( ! this.lppUserChanged ) {
              this.updateDeviceData( message.UseCase! );
              this.limits[message.SKI][message.UseCase!].FSDuration = message.Value ?? 0;
            }
            break;
          }
          case MessageType.GetConsumptionNominalMax: {
            this.consumptionNominalMax[message.SKI] = message.Value ?? 0;
            break;
          }
          case MessageType.GetProductionNominalMax: {
            this.productionNominalMax[message.SKI] = message.Value ?? 0;
            break;
          }
          case MessageType.GetConsumptionHeartbeat: {
            this.updateDeviceData( message.UseCase! );
            this.sendNotification( MessageType.GetAllData, message.UseCase );
            this.consumptionHeartbeat = true;
            setTimeout( () => this.consumptionHeartbeat = false, 1000 );
            break;
          }
          case MessageType.GetProductionHeartbeat: {
            this.updateDeviceData( message.UseCase! );
            this.sendNotification( MessageType.GetAllData, message.UseCase );
            this.productionHeartbeat = true;
            setTimeout( () => this.productionHeartbeat = false, 1000 );
            break;
          }
          case MessageType.GetPowerLimitationFactor: {
            this.updateDeviceData( message.UseCase! );
            this.monitorings[message.SKI][message.UseCase!].PowerLimitationFactor = message.Value ?? 0;
            break;
          }
	        case MessageType.GetPower: {
            this.updateDeviceData( message.UseCase! );
            this.monitorings[message.SKI][message.UseCase!].Power = message.Value ?? 0;
            break;
          }
        	case MessageType.GetPowerPerPhase: {
            this.updateDeviceData( message.UseCase! );
            this.monitorings[message.SKI][message.UseCase!].PowerPerPhase = message.Values ?? [0, 0, 0];
            break;
          }
        	case MessageType.GetEnergyFeedIn: {
            this.updateDeviceData( message.UseCase! );
            this.monitorings[message.SKI][message.UseCase!].EnergyFeedIn = message.Value ?? 0;
            break;
          }
        	case MessageType.GetEnergyConsumed: {
            this.updateDeviceData( message.UseCase! );
            this.monitorings[message.SKI][message.UseCase!].EnergyConsumed = message.Value ?? 0;
            break;
          }
        	case MessageType.GetCurrentPerPhase: {
            this.updateDeviceData( message.UseCase! );
            this.monitorings[message.SKI][message.UseCase!].CurrentPerPhase = message.Values ?? [0, 0, 0];
            break;
          }
        	case MessageType.GetVoltagePerPhase: {
            this.updateDeviceData( message.UseCase! );
            this.monitorings[message.SKI][message.UseCase!].VoltagePerPhase = message.Values ?? [0, 0, 0];
            break;
          }
        	case MessageType.GetFrequency: {
            this.updateDeviceData( message.UseCase! );
            this.monitorings[message.SKI][message.UseCase!].Frequency = message.Value ?? 0;
            break;
          }
        }   
      }
    }

    private updateDeviceData( useCase: string ) {
      if ( useCase == "LPC" || useCase == "LPP" ) {
        if ( ! this.limits[this.selectedSki] )
          this.limits[this.selectedSki] = {};
        if ( ! this.limits[this.selectedSki][useCase] )
          this.limits[this.selectedSki][useCase] = {} as Limits;
      }
      else if ( useCase == "MGCP" || useCase == "MPC" ) {
        if ( ! this.monitorings[this.selectedSki] )
          this.monitorings[this.selectedSki] = {};
        if ( ! this.monitorings[this.selectedSki][useCase] )
          this.monitorings[this.selectedSki][useCase] = {} as Monitorings;
      }
    }

    private syncEntities( entities: EntityInfo[] | undefined ) {
      if ( undefined == entities || 0 == entities.length ) {
        this.remoteEntities = [];
        return;
      }

      [...this.remoteEntities].forEach( re => {
        var indx = entities.findIndex( e => e.Address == re.Address );
        if ( -1 == indx ) {
          var indx2 = this.remoteEntities!.findIndex( e => e.Address == re.Address );
          this.remoteEntities = this.remoteEntities.splice( indx2, 1 );
        }
      } )

      entities.forEach( e => {
        var indx = this.remoteEntities.findIndex( re => re.Address == e.Address );
        if ( -1 == indx ) {
          this.remoteEntities.push( e );
        }
        else {
          this.remoteEntities[indx].Features = e.Features;
        }
      } );
    }

    private syncUseCases( useCaseInfos: UseCaseInfos | undefined ) {
      Object.keys( useCaseInfos! ).forEach( ski => {
        var infosOld = this.useCaseInfos[ski] ?? [];
        var infosNew = useCaseInfos![ski];

        [...infosOld].forEach( (infoOld, index) => {
            if ( undefined == infosNew.find( infoNew => infoNew.Actor == infoOld.Actor ) ) {
              infosOld = infosOld.splice( index, 1 );
            }
        } );

        infosNew.forEach( infoNew => {
          var infoOld = infosOld.find( infoOld => infoOld.Actor == infoNew.Actor )
          if ( undefined == infoOld )
            infosOld.push( infoNew );
          else
            infoOld.Names = infoNew.Names;    
        } );
        
        this.useCaseInfos[ski] = infosOld;
      } );
    }

    public serviceSelected() {
      this.selectedEntity = undefined;
      this.sendNotification( MessageType.SelectService, this.selectedSki );
    }

    private sendNotification( type: MessageType, param: string = "" ) {
      let command: Message = {
        SKI:  this.selectedSki,
        Type: type,
        Text: param
      };

      this.socket!.send( JSON.stringify( command ) );
    }

    private sendText( text: string ) {
      let command: Message = {
        SKI:  this.selectedSki,
        Type: MessageType.Text,
        Text: text,
      };

      this.socket!.send( JSON.stringify( command ) );
    }

    private sendLimits( type: MessageType, value: Limits ) {
      let command: Message = {
        SKI:   this.selectedSki,
        Type:  type,
        Limit: value
      };

      this.socket!.send( JSON.stringify( command ) );
    }

    private sendValue( type: MessageType, value: number ) {
      let command: Message = {
        SKI:   this.selectedSki,
        Type:  type,
        Value: value
      };

      this.socket!.send( JSON.stringify( command ) );
    }

    // An empty field means no limit. It cannot be written back, as eebus-go
    // would encode +Inf as a large negative number.
    public isUnlimited( value: number | string | null | undefined ) {
      return value === null || value === undefined || value === "";
    }

    public unlimitedHint( value: number | string | null | undefined ) {
      return this.isUnlimited( value ) ? "Enter a value to set a limit" : "";
    }

    private setReportedLimit( message: Message ) {
      if ( ! this.reportedLimits[message.SKI] )
        this.reportedLimits[message.SKI] = {};

      this.reportedLimits[message.SKI][message.UseCase!] = {
        IsActive: message.Limit?.IsActive ?? false,
        Value:    message.Unlimited ? null : message.Limit?.Value ?? 0
      };
    }

    public saveSettings() {
      try {
        localStorage.setItem( settingsKey, JSON.stringify( {
          installedPv: this.installedPv,
          ledRules:    this.ledRules
        } as Settings ) );
      }
      catch {
        // settings are a convenience only
      }
    }

    public get hasInstalledPv(): boolean {
      return 0 < Number( this.installedPv[this.selectedSki] );
    }

    // Allowed feed-in in % of installed PV power as confirmed by the device,
    // 100 if not limited, undefined if unknown.
    public get reportedFeedIn(): number | undefined {
      var limit = this.reportedLimits[this.selectedSki]?.['LPP'];
      if ( ! limit )
        return undefined;
      if ( ! limit.IsActive || null === limit.Value )
        return 100;
      if ( ! this.hasInstalledPv )
        return undefined;

      // devices like the Weidmueller ERC report feed-in limits as negative values
      return Math.min( 100, 100 * Math.abs( limit.Value ) / Number( this.installedPv[this.selectedSki] ) );
    }

    public isFeedInStep( pct: number ): boolean {
      var feedIn = this.reportedFeedIn;
      return undefined !== feedIn && Math.abs( feedIn - pct ) < 0.5;
    }

    public feedInHint( pct: number ): string {
      if ( 100 <= pct )
        return "Deactivate the feed-in limit";

      return "Limit feed-in to " + this.formatted( Math.round( Number( this.installedPv[this.selectedSki] ) * pct / 100 ) ) + " W";
    }

    // Fills Active and Dimmed Value and sends the limit like the Set button.
    public setFeedIn( pct: number ) {
      if ( ! this.hasInstalledPv )
        return;

      var installed = Number( this.installedPv[this.selectedSki] );
      var ls = this.limits[this.selectedSki]['LPP'];

      // 100 % means no limitation: deactivate instead of limiting to full power
      ls.IsActive = pct < 100;
      // feed-in limits are negative, as also reported back by the device
      ls.Value    = -Math.round( installed * pct / 100 ) || 0;

      this.setProductionLimit();
    }

    // §14a EnWG minimum power a controllable consumer must still be allowed
    public readonly consumptionLimitValue = 4200;

    public isConsumptionLimited( active: boolean ): boolean {
      var limit = this.reportedLimits[this.selectedSki]?.['LPC'];
      if ( ! limit )
        return false;

      return active ? limit.IsActive && limit.Value == this.consumptionLimitValue : ! limit.IsActive;
    }

    // Fills Active and Dimmed Value and sends the limit like the Set button.
    public setConsumption( active: boolean ) {
      var ls = this.limits[this.selectedSki]['LPC'];

      ls.IsActive = active;
      ls.Value    = this.consumptionLimitValue;

      this.setConsumptionLimit();
    }

    public ledState( rule: LedRule ): 'on' | 'off' | 'unknown' {
      if ( 'LPC' == rule.Source ) {
        var limit = this.reportedLimits[this.selectedSki]?.['LPC'];
        if ( ! limit )
          return 'unknown';
        return limit.IsActive ? 'on' : 'off';
      }

      if ( 'LPP' == rule.Source ) {
        var feedIn = this.reportedFeedIn;
        if ( undefined === feedIn )
          return 'unknown';
        if ( 100 <= feedIn )
          return 'off';

        // the output for the highest step not exceeding the allowed feed-in
        var steps = this.ledRules.filter( r => 'LPP' == r.Source ).map( r => r.Step );
        var allowed = steps.filter( step => step <= feedIn! + 0.5 );
        var step = allowed.length ? Math.max( ...allowed ) : Math.min( ...steps );

        return rule.Step == step ? 'on' : 'off';
      }

      return 'off';
    }

    public ledHint( rule: LedRule ): string {
      switch ( rule.Source ) {
        case 'LPC': return rule.Name + ": consumption limit active";
        case 'LPP': return rule.Name + ": feed-in limited to " + rule.Step + " %" + ( this.hasInstalledPv ? "" : " (enter installed PV power)" );
        default:    return rule.Name + ": not used";
      }
    }

    public onLPCUserChanged() {
      this.lpcUserChanged = true;
    }

    public onLPPUserChanged() {
      this.lppUserChanged = true;
    }

    public setConsumptionLimit() {
      if ( ! this.socket )
        return;
      
      this.sendLimits( MessageType.SetConsumptionLimit, this.limits[this.selectedSki]['LPC'] );
      this.lpcUserChanged = false;
    }

    public setProductionLimit() {
      if ( ! this.socket )
        return;
      
      this.sendLimits( MessageType.SetProductionLimit, this.limits[this.selectedSki]['LPP'] );
      this.lppUserChanged = false;
    }

    public setConsumptionFailsafeLimit() {
      if ( ! this.socket )
        return;
      
      this.sendValue( MessageType.SetConsumptionFailsafeValue, this.limits[this.selectedSki]['LPC'].FSValue! );
      this.lpcUserChanged = false;
    }

    public setConsumptionFailsafeDuration() {
      if ( ! this.socket )
        return;
      
      this.sendValue( MessageType.SetConsumptionFailsafeDuration, this.limits[this.selectedSki]['LPC'].FSDuration );
      this.lpcUserChanged = false;
    }

    public setProductionFailsafeLimit() {
      if ( ! this.socket )
        return;
      
      this.sendValue( MessageType.SetProductionFailsafeValue, this.limits[this.selectedSki]['LPP'].FSValue! );
      this.lppUserChanged = false;
    }

    public setProductionFailsafeDuration() {
      if ( ! this.socket )
        return;
      
      this.sendValue( MessageType.SetProductionFailsafeDuration, this.limits[this.selectedSki]['LPP'].FSDuration );
      this.lppUserChanged = false;
    }

    public toggleConsumptionHeartbeat() {
      if ( ! this.socket )
        return;

      if ( this.consumptionHeartbeatEnabled )
        this.sendValue( MessageType.StopConsumptionHeartbeat, 0 );
      else
        this.sendValue( MessageType.StartConsumptionHeartbeat, 0 );

      this.consumptionHeartbeatEnabled = ! this.consumptionHeartbeatEnabled;
    }

    public toggleProductionHeartbeat() {
      if ( ! this.socket )
        return;

      if ( this.productionHeartbeatEnabled )
        this.sendValue( MessageType.StopProductionHeartbeat, 0 );
      else
        this.sendValue( MessageType.StartProductionHeartbeat, 0 );

      this.productionHeartbeatEnabled = ! this.productionHeartbeatEnabled;
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
  .read-the-docs {
    color: #888;
  }
  .header-frame {
    display: inline-table;
    margin-bottom: 10px;
  }
  .header {
    display: grid;
    grid-template-columns: 75fr 25fr;
    column-gap: 25px;
  }
  h1 {
    margin-top: 0.1em;
  }
  .qrcode-text {
    width: 370px;
    text-align: left;
    word-break: break-all;
  }
  .qrcode {
    width: 130px;
    height: 100%;
  }
  .usecases {
    display: grid;
    grid-template-columns: 50fr 50fr;
    column-gap: 25px;
  }
  .three-lines {
    grid-column-start: 3;
    grid-row-start: 1;
    grid-row-end: 4;
  }
  .form-line3 {
    display: grid;
    grid-template-columns: 50fr 30fr 20fr;
    column-gap: 10px;
  }
  .form-line3 label {
    align-content: center;
    text-align: left;
  }
  .form-line3 input {
    font-size: initial;
    width: 100px;
    align-self: center;
  }
  .form-line3 button {
    line-height: 5px;
    height: 100%;
  }
  
  .form-line2 {
    display: grid;
    grid-template-columns: 50fr 50fr;
    column-gap: 10px;
  }
  .form-line2 label {
    align-content: center;
    text-align: left;
  }

  .pulse {
    font-size: 25px;
  }

  .heartbeat {
    animation-name: heartbeat;
    animation-duration: 1s;
    /* animation-iteration-count: infinite; */
  }

  @keyframes heartbeat {
    from {
      color: rgb(0,255,0);
    }

    25% {
      color: rgb(0,255,0);
    }

    50% {
      color: rgb(0,127,0);
    }

    75% {
      color: rgb(0,63,0);
    }

    to {
      color: rgb(0,0,0);
    }
  }

  .quick-buttons {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: 6px;
    margin-bottom: 12px;
  }
  .quick-buttons button {
    white-space: nowrap;
    padding: 0.3em 0.9em;
  }
  .quick-buttons button:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
  .hint {
    color: rgb(191,96,0);
    font-size: 0.85em;
    align-self: center;
  }
  .form-line3 input.missing {
    border: 2px solid rgb(191,96,0);
  }
  .quick-buttons button.selected {
    border-color: rgb(0,191,0);
    box-shadow: 0 0 4px rgb(0,191,0);
  }

  .outputs {
    text-align: left;
  }
  .leds {
    display: flex;
    gap: 24px;
  }
  .led-item {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .led {
    display: inline-block;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    border: 1px solid #555;
    background: rgb(40,40,40);
  }
  .led.on {
    background: rgb(0,255,0);
    box-shadow: 0 0 8px rgb(0,255,0);
  }
  .led.unknown {
    background: transparent;
    border-style: dashed;
  }
  .led-config {
    margin-top: 10px;
  }
  .led-config-grid {
    display: grid;
    grid-template-columns: 40px max-content max-content;
    gap: 6px 10px;
    align-items: center;
    margin-top: 6px;
  }
  .led-config-grid input {
    width: 60px;
  }

  .devices {
    display: grid;
    grid-template-columns: 20fr 80fr;
    column-gap: 10px;
  }
  .device-select-label {
    text-align: left;
    line-height: 2.2em;
  }
</style>
