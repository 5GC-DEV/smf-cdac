package ngapType

// Need to import "github.com/free5gc/aper" if it uses "aper"

type QosFlowWithCauseItem struct {
	QosFlowIdentifier QosFlowIdentifier
	Cause             Cause                                                 `aper:"valueLB:0,valueUB:5"`
	IEExtensions      *ProtocolExtensionContainerQosFlowWithCauseItemExtIEs `aper:"optional"`
}
