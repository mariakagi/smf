package context

import (
	"fmt"
	"net"
	"os"

	"free5gc/lib/openapi/Nnrf_NFDiscovery"
	"free5gc/lib/openapi/Nnrf_NFManagement"
	"free5gc/lib/openapi/Nudm_SubscriberDataManagement"
	"free5gc/src/smf/factory"
	"free5gc/src/smf/logger"

	"free5gc/lib/openapi/models"
	"free5gc/lib/pfcp/pfcpType"
	"free5gc/lib/pfcp/pfcpUdp"

	"github.com/google/uuid"
)

func init() {
	smfContext.NfInstanceID = uuid.New().String()
}

var smfContext SMFContext

type SMFContext struct {
	Name         string
	ServerIPv4	string

	NfInstanceID string

	URIScheme   models.UriScheme
	HTTPAddress string
	HTTPPort    int
	CPNodeID    pfcpType.NodeID

	UDMProfile models.NfProfile

	SnssaiInfos []models.SnssaiSmfInfoItem

	UPNodeIDs []pfcpType.NodeID
	Key       string
	PEM       string
	KeyLog    string

	UESubNet      *net.IPNet
	UEAddressTemp net.IP

	NrfUri                         string
	NFManagementClient             *Nnrf_NFManagement.APIClient
	NFDiscoveryClient              *Nnrf_NFDiscovery.APIClient
	SubscriberDataManagementClient *Nudm_SubscriberDataManagement.APIClient

	UserPlaneInformation UserPlaneInformation
	//*** For ULCL ** //
	ULCLSupport     bool
	UERoutingPaths  map[string][]factory.Path
	UERoutingGraphs map[string]*UEDataPathGraph
}

func AllocUEIP() net.IP {
	smfContext.UEAddressTemp[3]++
	return smfContext.UEAddressTemp
}

func InitSmfContext(config *factory.Config) {
	if config == nil {
		logger.CtxLog.Infof("Config is nil")
	}

	logger.CtxLog.Infof("smfconfig Info: Version[%s] Description[%s]", config.Info.Version, config.Info.Description)
	configuration := config.Configuration
	if configuration.SmfName != "" {
		smfContext.Name = configuration.SmfName
	}

	smfContext.ServerIPv4 = os.Getenv(configuration.ServerIPv4)
	if smfContext.ServerIPv4 == "" {
		logger.CtxLog.Warn("Problem parsing ServerIPv4 address from ENV Variable. Trying to parse it as string.")
		smfContext.ServerIPv4 = configuration.ServerIPv4
		if smfContext.ServerIPv4 == "" {
			logger.CtxLog.Warn("Error parsing ServerIPv4 address as string. Using the localhost address as default.")
			smfContext.ServerIPv4 = "127.0.0.1"
		}
	}

	sbi := configuration.Sbi
	smfContext.URIScheme = models.UriScheme(sbi.Scheme)
	smfContext.HTTPAddress = "127.0.0.1" // default localhost
	smfContext.HTTPPort = 29502          // default port
	if sbi != nil {
		if sbi.IPv4Addr != "" {
			smfContext.HTTPAddress = sbi.IPv4Addr
		}
		if sbi.Port != 0 {
			smfContext.HTTPPort = sbi.Port
		}

		if tls := sbi.TLS; tls != nil {
			smfContext.Key = tls.Key
			smfContext.PEM = tls.PEM
		}
	}
	if configuration.NrfUri != "" {
		smfContext.NrfUri = configuration.NrfUri
	} else {
		logger.CtxLog.Error("NRF Uri is empty! Using localhost as NRF IPv4 address.")
		smfContext.NrfUri = fmt.Sprintf("%s://%s:%d", smfContext.URIScheme, "127.0.0.1", 29510)
	}

	if pfcp := configuration.PFCP; pfcp != nil {
		if pfcp.Port == 0 {
			pfcp.Port = pfcpUdp.PFCP_PORT
		}
		pfcp.Addr = os.Getenv(pfcp.Addr)
		if pfcp.Addr == "" {
			logger.CtxLog.Warn("Problem parsing PFCP IPv4 address from ENV Variable. Using the localhost address as default.")
			pfcp.Addr = "127.0.0.1"
		}
		addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", pfcp.Addr, pfcp.Port))
		if err != nil {
			logger.CtxLog.Warnf("PFCP Parse Addr Fail: %v", err)
		}

		smfContext.CPNodeID.NodeIdType = 0
		smfContext.CPNodeID.NodeIdValue = addr.IP.To4()
	}

	_, ipNet, err := net.ParseCIDR(configuration.UESubnet)
	if err != nil {
		logger.InitLog.Errorln(err)
	}
	smfContext.UESubNet = ipNet
	smfContext.UEAddressTemp = ipNet.IP

	// Set client and set url
	ManagementConfig := Nnrf_NFManagement.NewConfiguration()
	ManagementConfig.SetBasePath(SMF_Self().NrfUri)
	smfContext.NFManagementClient = Nnrf_NFManagement.NewAPIClient(ManagementConfig)

	NFDiscovryConfig := Nnrf_NFDiscovery.NewConfiguration()
	NFDiscovryConfig.SetBasePath(SMF_Self().NrfUri)
	smfContext.NFDiscoveryClient = Nnrf_NFDiscovery.NewAPIClient(NFDiscovryConfig)

	smfContext.ULCLSupport = configuration.ULCL

	smfContext.SnssaiInfos = configuration.SNssaiInfo

	processUPTopology(&configuration.UserPlaneInformation)

	SetupNFProfile(config)
}

func InitSMFUERouting(routingConfig *factory.RoutingConfig) {

	if routingConfig == nil {
		logger.CtxLog.Infof("Routing Config is nil")
	}

	logger.CtxLog.Infof("ue routing config Info: Version[%s] Description[%s]",
		routingConfig.Info.Version, routingConfig.Info.Description)

	UERoutingInfo := routingConfig.UERoutingInfo
	smfContext.UERoutingPaths = make(map[string][]factory.Path)
	smfContext.UERoutingGraphs = make(map[string]*UEDataPathGraph)

	for _, routingInfo := range UERoutingInfo {

		supi := routingInfo.SUPI

		smfContext.UERoutingPaths[supi] = routingInfo.PathList
	}

	for supi := range smfContext.UERoutingPaths {

		graph, err := NewUEDataPathGraph(supi)

		if err != nil {
			logger.CtxLog.Warnln(err)
			continue
		}

		smfContext.UERoutingGraphs[supi] = graph
	}

}

func SMF_Self() *SMFContext {
	return &smfContext
}

func GetUserPlaneInformation() *UserPlaneInformation {
	return &smfContext.UserPlaneInformation
}
