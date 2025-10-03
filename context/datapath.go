// SPDX-FileCopyrightText: 2022-present Intel Corporation

// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>

// Copyright 2019 free5GC.org

//

// SPDX-License-Identifier: Apache-2.0

package context

import (
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/5GC-DEV/openapi-cdac/models"
	"github.com/5GC-DEV/util-cdac/util_3gpp"
	"github.com/omec-project/smf/logger"
	"github.com/omec-project/smf/qos"
	"github.com/omec-project/smf/util"
)

// GTPTunnel represents the GTP tunnel information

type GTPTunnel struct {
	SrcEndPoint *DataPathNode

	DestEndPoint *DataPathNode

	PDR map[string]*PDR

	TEID uint32
}

type DataPathNode struct {
	UPF *UPF

	// DataPathToAN *DataPathDownLink

	// DataPathToDN map[string]*DataPathUpLink //uuid to DataPathLink

	UpLinkTunnel *GTPTunnel

	DownLinkTunnel *GTPTunnel

	// for UE Routing Topology

	// for special case:

	// branching & leafnode

	// InUse                bool

	IsBranchingPoint bool

	// DLDataPathLinkForPSA *DataPathUpLink

	// BPUpLinkPDRs         map[string]*DataPathDownLink // uuid to UpLink

}

type DataPath struct {

	// Data Path Double Link List

	FirstDPNode *DataPathNode

	// meta data

	Destination Destination

	Activated bool

	IsDefaultPath bool

	HasBranchingPoint bool
}

type DataPathPool map[int64]*DataPath

type Destination struct {
	DestinationIP string

	DestinationPort string

	Url string
}

func NewDataPathNode() *DataPathNode {

	node := &DataPathNode{

		UpLinkTunnel: &GTPTunnel{PDR: make(map[string]*PDR)},

		DownLinkTunnel: &GTPTunnel{PDR: make(map[string]*PDR)},
	}

	return node

}

func NewDataPath() *DataPath {

	dataPath := &DataPath{

		Destination: Destination{

			DestinationIP: "",

			DestinationPort: "",

			Url: "",
		},
	}

	return dataPath

}

func NewDataPathPool() DataPathPool {

	pool := make(map[int64]*DataPath)

	return pool

}

func (node *DataPathNode) AddNext(next *DataPathNode) {

	node.DownLinkTunnel.SrcEndPoint = next

}

func (node *DataPathNode) AddPrev(prev *DataPathNode) {

	node.UpLinkTunnel.SrcEndPoint = prev

}

func (node *DataPathNode) Next() *DataPathNode {

	if node.DownLinkTunnel == nil {

		return nil

	}

	next := node.DownLinkTunnel.SrcEndPoint

	return next

}

func (node *DataPathNode) Prev() *DataPathNode {

	if node.UpLinkTunnel == nil {

		return nil

	}

	prev := node.UpLinkTunnel.SrcEndPoint

	return prev

}

func (node *DataPathNode) ActivateUpLinkTunnel(smContext *SMContext) error {

	var err error

	var pdr *PDR

	var flowQer *QER

	logger.CtxLog.Debugln("in ActivateUpLinkTunnel")

	logger.CtxLog.Infof("[DP][ActivateUpLinkTunnel][Enter] Supi=%s  NodePtr=%p", smContext.Supi, node)

	node.UpLinkTunnel.SrcEndPoint = node.Prev()

	node.UpLinkTunnel.DestEndPoint = node

	destUPF := node.UPF

	// Iterate through PCC Rules to install PDRs

	pccRuleUpdate := smContext.SmPolicyUpdates[0].PccRuleUpdate

	if pccRuleUpdate != nil {

		addRules := pccRuleUpdate.GetAddPccRuleUpdate()

		logger.CtxLog.Infof("[DP][ActivateUpLinkTunnel] Supi=%s Found %d PCC Rules", smContext.Supi, len(addRules))

		for name, rule := range addRules {

			logger.CtxLog.Infof("[DP][ActivateUpLinkTunnel] [PDR][Build] Rule=%s Supi=%s ", name, smContext.Supi)

			if pdr, err = destUPF.BuildCreatePdrFromPccRule(rule); err == nil {

				logger.CtxLog.Infof("[DP][ActivateUpLinkTunnel][PDR][Success] Rule=%s PDRID=%d Supi=%s", name, pdr.PDRID, smContext.Supi)

				// Add PCC Rule Qos Data QER

				if flowQer, err = node.CreatePccRuleQer(smContext, rule.RefQosData[0], rule.RefTcData[0]); err == nil {

					logger.CtxLog.Infof("[DP][ActivateUpLinkTunnel][QER][Success] Rule=%s QERID=%d QosRef=%s TcRef=%s Supi=%s", name, flowQer.QERID, rule.RefQosData[0], rule.RefTcData[0], smContext.Supi)

					pdr.QER = append(pdr.QER, flowQer)

				} else {

					logger.CtxLog.Errorf("[DP][ActivateUpLinkTunnel][QER][Error] Rule=%s err=%v Supi=%s", name, err, smContext.Supi)

				}

				// Set PDR in Tunnel

				node.UpLinkTunnel.PDR[name] = pdr

			} else {

				logger.CtxLog.Errorf("[DP][ActivateUpLinkTunnel][PDR][Error] Rule=%s err=%v Supi=%s", name, err, smContext.Supi)

			}

		}

	} else {

		// Default PDR

		logger.CtxLog.Infof("[DP][ActivateUpLinkTunnel][PDR][Default] Installing default PDR for UPF=%s Supi=%s", node.UPF.NodeID.ResolveNodeIdToIp().String(), smContext.Supi)

		if pdr, err = destUPF.AddPDR(); err != nil {

			logger.CtxLog.Errorln("[DP][ActivateUpLinkTunnel][Error]Supi=%s  in ActivateUpLinkTunnel UPF IP:", smContext.Supi, node.UPF.NodeID.ResolveNodeIdToIp().String())

			logger.CtxLog.Errorln("allocate PDR error:", err)

			return fmt.Errorf("add PDR failed: %s", err)

		} else {

			logger.CtxLog.Infof("[DP][ActivateUpLinkTunnel][PDR][Success] Default PDRID=%d installed for UPF=%s", pdr.PDRID, node.UPF.NodeID.ResolveNodeIdToIp().String())

			node.UpLinkTunnel.PDR["default"] = pdr

		}

	}

	if err = smContext.PutPDRtoPFCPSession(destUPF.NodeID, node.UpLinkTunnel.PDR); err != nil {

		logger.CtxLog.Errorf("[DP][ActivateUpLinkTunnel][Error] PutPDRtoPFCP UPF=%s err=%v", node.UPF.NodeID.ResolveNodeIdToIp().String(), err)

		logger.CtxLog.Errorln("put PDR Error:", err)

		return err

	}

	logger.CtxLog.Infof("[DP][ActivateUpLinkTunnel][Success] Supi=%s Installed %d PDR(s) into PFCP Session for UPF=%s", smContext.Supi, len(node.UpLinkTunnel.PDR), node.UPF.NodeID.ResolveNodeIdToIp().String())

	logger.CtxLog.Infof("[DP][ActivateUpLinkTunnel][Exit] Supi=%s UPF=%s InstalledPDRs=%d", smContext.Supi, node.UPF.NodeID.ResolveNodeIdToIp().String(), len(node.UpLinkTunnel.PDR))

	return nil

}

func (node *DataPathNode) ActivateDownLinkTunnel(smContext *SMContext) error {

	var err error

	var pdr *PDR

	var flowQer *QER

	logger.CtxLog.Infof("[DP][ActivateDownLinkTunnel][Enter] Supi=%s  UPF=%s NodePtr=%p", smContext.Supi, node.UPF.NodeID.ResolveNodeIdToIp().String(), node)

	node.DownLinkTunnel.SrcEndPoint = node.Next()

	node.DownLinkTunnel.DestEndPoint = node

	destUPF := node.UPF

	// Iterate through PCC Rules to install PDRs

	pccRuleUpdate := smContext.SmPolicyUpdates[0].PccRuleUpdate

	if pccRuleUpdate != nil {

		addRules := pccRuleUpdate.GetAddPccRuleUpdate()

		logger.CtxLog.Infof("[DP][ActivateDownLinkTunnel] UPF=%s Found %d PCC Rules Supi=%s ", node.UPF.NodeID.ResolveNodeIdToIp().String(), len(addRules), smContext.Supi)

		for name, rule := range addRules {

			logger.CtxLog.Infof("[DP][ActivateDownLinkTunnel][PDRBuild] Supi=%s Rule=%s UPF=%s", smContext.Supi, name, node.UPF.NodeID.ResolveNodeIdToIp().String())

			if pdr, err = destUPF.BuildCreatePdrFromPccRule(rule); err == nil {

				logger.CtxLog.Infof("[DP][ActivateDownLinkTunnel][PDR][Success] Supi=%s  Rule=%s PDRID=%d", smContext.Supi, name, pdr.PDRID)

				// Add PCC Rule Qos Data QER

				if flowQer, err = node.CreatePccRuleQer(smContext, rule.RefQosData[0], rule.RefTcData[0]); err == nil {

					logger.CtxLog.Infof("[DP][ActivateDownLinkTunnel][[QER][Success] Supi=%s  Rule=%s QERID=%d QosRef=%s TcRef=%s", smContext.Supi, name, flowQer.QERID, rule.RefQosData[0], rule.RefTcData[0])

					pdr.QER = append(pdr.QER, flowQer)

				} else {

					logger.CtxLog.Errorf("[DP][ActivateDownLinkTunnel][[QER][Error] Supi=%s Rule=%s err=%v", smContext.Supi, name, err)

				}

				// Set PDR in Tunnel

				node.DownLinkTunnel.PDR[name] = pdr

			} else {

				logger.CtxLog.Errorf("[DP][ActivateDownLinkTunnel][[PDR][Error] Supi=%s Rule=%s err=%v", smContext.Supi, name, err)

			}

		}

	} else {

		// Default PDR

		logger.CtxLog.Infof("[DP][ActivateDownLinkTunnel][PDR][Default] Supi=%s Installing default PDR for UPF=%s", smContext.Supi, node.UPF.NodeID.ResolveNodeIdToIp().String())

		if pdr, err = destUPF.AddPDR(); err != nil {

			logger.CtxLog.Errorln("[DP][ActivateDownLinkTunnel] Supi=%s  in ActivateDownLinkTunnel UPF IP:", smContext.Supi, node.UPF.NodeID.ResolveNodeIdToIp().String())

			logger.CtxLog.Errorln("allocate PDR Error:", err)

			return fmt.Errorf("add PDR failed: %s", err)

		} else {

			logger.CtxLog.Infof("[DP][ActivateDownLinkTunnel] [PDR][Success] Supi=%s Default PDRID=%d installed for UPF=%s", smContext.Supi, pdr.PDRID, node.UPF.NodeID.ResolveNodeIdToIp().String())

			node.DownLinkTunnel.PDR["default"] = pdr

		}

	}

	// Put PDRs in PFCP session

	if err = smContext.PutPDRtoPFCPSession(destUPF.NodeID, node.DownLinkTunnel.PDR); err != nil {

		logger.CtxLog.Errorf("[DP][ActivateDownLinkTunnel][Error] PutPDRtoPFCP UPF=%s err=%v", node.UPF.NodeID.ResolveNodeIdToIp().String(), err)

		return err

	}

	logger.CtxLog.Infof("[DP][ActivateDownLinkTunnel][Success] Supi=%s Installed %d PDR(s) into PFCP Session for UPF=%s", smContext.Supi, len(node.DownLinkTunnel.PDR), node.UPF.NodeID.ResolveNodeIdToIp().String())

	logger.CtxLog.Infof("[DP][ActivateDownLinkTunnel][Exit] Supi=%s  UPF=%s InstalledPDRs=%d", smContext.Supi, node.UPF.NodeID.ResolveNodeIdToIp().String(), len(node.DownLinkTunnel.PDR))

	return nil

}

func (node *DataPathNode) DeactivateUpLinkTunnel(smContext *SMContext) {

	for name, pdr := range node.UpLinkTunnel.PDR {

		if pdr != nil {

			logger.CtxLog.Infof("deactivated UpLinkTunnel PDR name[%v], id[%v]", name, pdr.PDRID)

			// Remove PDR from PFCP Session

			smContext.RemovePDRfromPFCPSession(node.UPF.NodeID, pdr)

			// Remove of UPF

			err := node.UPF.RemovePDR(pdr)

			if err != nil {

				logger.CtxLog.Warnln("deactivated UpLinkTunnel", err)

			}

			if far := pdr.FAR; far != nil {

				err = node.UPF.RemoveFAR(far)

				if err != nil {

					logger.CtxLog.Warnln("deactivated UpLinkTunnel", err)

				}

				bar := far.BAR

				if bar != nil {

					err = node.UPF.RemoveBAR(bar)

					if err != nil {

						logger.CtxLog.Warnln("deactivated UpLinkTunnel", err)

					}

				}

			}

			if qerList := pdr.QER; qerList != nil {

				for _, qer := range qerList {

					if qer != nil {

						err = node.UPF.RemoveQER(qer)

						if err != nil {

							logger.CtxLog.Warnln("deactivated UpLinkTunnel", err)

						}

					}

				}

			}

		}

	}

	node.DownLinkTunnel = &GTPTunnel{}

}

func (node *DataPathNode) DeactivateDownLinkTunnel(smContext *SMContext) {

	for name, pdr := range node.DownLinkTunnel.PDR {

		if pdr != nil {

			logger.CtxLog.Infof("deactivated DownLinkTunnel PDR name[%v], id[%v]", name, pdr.PDRID)

			// Remove PDR from PFCP Session

			smContext.RemovePDRfromPFCPSession(node.UPF.NodeID, pdr)

			// Remove from UPF

			err := node.UPF.RemovePDR(pdr)

			if err != nil {

				logger.CtxLog.Warnln("deactivated DownLinkTunnel", err)

			}

			if far := pdr.FAR; far != nil {

				err = node.UPF.RemoveFAR(far)

				if err != nil {

					logger.CtxLog.Warnln("deactivated DownLinkTunnel", err)

				}

				bar := far.BAR

				if bar != nil {

					err = node.UPF.RemoveBAR(bar)

					if err != nil {

						logger.CtxLog.Warnln("deactivated DownLinkTunnel", err)

					}

				}

			}

			if qerList := pdr.QER; qerList != nil {

				for _, qer := range qerList {

					if qer != nil {

						err = node.UPF.RemoveQER(qer)

						if err != nil {

							logger.CtxLog.Warnln("deactivated UpLinkTunnel", err)

						}

					}

				}

			}

		}

	}

	node.DownLinkTunnel = &GTPTunnel{}

}

func (node *DataPathNode) GetUPFID() (id string, err error) {

	node_ip := node.GetNodeIP()

	var exist bool

	if id, exist = smfContext.UserPlaneInformation.UPFsIPtoID[node_ip]; !exist {

		AllocateUPFID()

		if id, exist = smfContext.UserPlaneInformation.UPFsIPtoID[node_ip]; !exist {

			err = fmt.Errorf("UPNode IP %s doesn't exist in smfcfg.yaml", node_ip)

			return "", err

		}

	}

	return id, nil

}

func (node *DataPathNode) GetNodeIP() (ip string) {

	ip = node.UPF.NodeID.ResolveNodeIdToIp().String()

	return

}

func (node *DataPathNode) IsANUPF() bool {

	if node.Prev() == nil {

		return true

	} else {

		return false

	}

}

func (node *DataPathNode) IsAnchorUPF() bool {

	if node.Next() == nil {

		return true

	} else {

		return false

	}

}

func (dataPathPool DataPathPool) GetDefaultPath() (dataPath *DataPath) {

	for _, path := range dataPathPool {

		if path.IsDefaultPath {

			dataPath = path

			return

		}

	}

	return

}

func (dataPath *DataPath) String() string {

	firstDPNode := dataPath.FirstDPNode

	var str string

	str += "DataPath Meta Information\n"

	str += "Activated: " + strconv.FormatBool(dataPath.Activated) + "\n"

	str += "IsDefault Path: " + strconv.FormatBool(dataPath.IsDefaultPath) + "\n"

	str += "Has Braching Point: " + strconv.FormatBool(dataPath.HasBranchingPoint) + "\n"

	str += "Destination IP: " + dataPath.Destination.DestinationIP + "\n"

	str += "Destination Port: " + dataPath.Destination.DestinationPort + "\n"

	str += "DataPath Routing Information\n"

	index := 1

	for curDPNode := firstDPNode; curDPNode != nil; curDPNode = curDPNode.Next() {

		str += strconv.Itoa(index) + "th Node in the Path\n"

		str += "Current UPF IP: " + curDPNode.GetNodeIP() + "\n"

		if curDPNode.Prev() != nil {

			str += "Previous UPF IP: " + curDPNode.Prev().GetNodeIP() + "\n"

		} else {

			str += "Previous UPF IP: None\n"

		}

		if curDPNode.Next() != nil {

			str += "Next UPF IP: " + curDPNode.Next().GetNodeIP() + "\n"

		} else {

			str += "Next UPF IP: None\n"

		}

		index++

	}

	return str

}

func (dataPath *DataPath) validateDataPathUpfStatus() error {

	firstDPNode := dataPath.FirstDPNode

	for curDataPathNode := firstDPNode; curDataPathNode != nil; curDataPathNode = curDataPathNode.Next() {

		logger.PduSessLog.Infof("nodes in Data Path [%v] and status [%v]",

			curDataPathNode.UPF.NodeID.ResolveNodeIdToIp().String(), curDataPathNode.UPF.UPFStatus.String())

		if curDataPathNode.UPF.UPFStatus != AssociatedSetUpSuccess {

			logger.PduSessLog.Errorf("UPF [%v] in DataPath not associated",

				curDataPathNode.UPF.NodeID.ResolveNodeIdToIp().String())

			return errors.New("UPF not associated in DataPath")

		}

	}

	return nil

}

func (dataPath *DataPath) ActivateUlDlTunnel(smContext *SMContext) error {

	firstDPNode := dataPath.FirstDPNode

	logger.PduSessLog.Infof("[DP][Enter][ActivateUlDlTunnel] Supi=%s DNN=%s Slice=%+v firstNode=%p ", smContext.Supi, smContext.Dnn, smContext.Snssai, firstDPNode)

	logger.PduSessLog.Infof("[DP][ActivateUlDlTunnel]  Full DataPath: %s", dataPath.String())

	// Activate Tunnels

	for curDataPathNode := firstDPNode; curDataPathNode != nil; curDataPathNode = curDataPathNode.Next() {

		logger.PduSessLog.Infof("[DP][ActivateUlDlTunnel][current DP Node IP:] UPF=%s NodePtr=%p Supi=%s", curDataPathNode.UPF.NodeID.ResolveNodeIdToIp().String(), curDataPathNode, smContext.Supi)

		if err := curDataPathNode.ActivateUpLinkTunnel(smContext); err != nil {

			logger.CtxLog.Warnln(err)

			logger.CtxLog.Errorf("[DP][ActivateUlDlTunnel][Error] UPF=%s err=%v Supi=%s", curDataPathNode.UPF.NodeID.ResolveNodeIdToIp().String(), err, smContext.Supi)

			return err

		}

		logger.PduSessLog.Infof("[DP][Success][Uplink] UPF=%s Supi=%s", curDataPathNode.UPF.NodeID.ResolveNodeIdToIp().String(), smContext.Supi)

		if err := curDataPathNode.ActivateDownLinkTunnel(smContext); err != nil {

			logger.CtxLog.Warnln(err)

			logger.CtxLog.Errorf("[DP][Error][Downlink] UPF=%s Supi=%s err=%v", curDataPathNode.UPF.NodeID.ResolveNodeIdToIp().String(), smContext.Supi, err)

			return err

		}

		logger.PduSessLog.Infof("[DP][Success][Downlink] UPF=%s supi=%s", curDataPathNode.UPF.NodeID.ResolveNodeIdToIp().String(), smContext.Supi)

	}

	return nil

}

func (dpNode *DataPathNode) CreatePccRuleQer(smContext *SMContext, qosData string, tcData string) (*QER, error) {

	smPolicyDec := smContext.SmPolicyUpdates[0].SmPolicyDecision

	refQos := qos.GetQoSDataFromPolicyDecision(smPolicyDec, qosData)

	tc := qos.GetTcDataFromPolicyDecision(smPolicyDec, tcData)

	logger.CtxLog.Info("[DP][CreatePccRuleQer][Enter] Supi=%s  UPF=%s qosData=%s tcData=%s",

		smContext.Supi, dpNode.UPF.NodeID.ResolveNodeIdToIp().String(), qosData, tcData)

	// Get Flow Status

	gateStatus := GateOpen

	if tc != nil && tc.FlowStatus == models.FlowStatus_DISABLED {

		gateStatus = GateClose

		logger.CtxLog.Info("[DP][CreatePccRuleQer][QER][GateStatus] FlowStatus=DISABLED → GateClosed  Supi=%s ", smContext.Supi)

	} else {

		logger.CtxLog.Info("[DP][CreatePccRuleQer][QER][GateStatus] FlowStatus=ENABLED → GateOpen  Supi=%s ", smContext.Supi)

	}

	var flowQER *QER

	if newQER, err := dpNode.UPF.AddQER(); err != nil {

		logger.CtxLog.Errorf("[DP][CreatePccRuleQer][Error] UPF=%s AddQER failed err=%v", dpNode.UPF.NodeID.ResolveNodeIdToIp().String(), err)

		logger.PduSessLog.Info("new QER failed Supi=%s", smContext.Supi)

		return nil, err

	} else {

		newQER.QFI.QFI = qos.GetQosFlowIdFromQosId(refQos.QosId)

		// Flow Status

		newQER.GateStatus = &GateStatus{

			ULGate: gateStatus,

			DLGate: gateStatus,
		}

		// Rates

		newQER.MBR = &MBR{

			ULMBR: util.BitRateTokbps(refQos.MaxbrUl),

			DLMBR: util.BitRateTokbps(refQos.MaxbrDl),
		}

		flowQER = newQER

		logger.CtxLog.Infof("[DP][QER][Success] QERID=%d QFI=%d ULMBR=%dkbps DLMBR=%dkbps GateStatus=(UL:%d DL:%d)  supi=%s",

			flowQER.QERID, flowQER.QFI.QFI,

			flowQER.MBR.ULMBR, flowQER.MBR.DLMBR,

			flowQER.GateStatus.ULGate, flowQER.GateStatus.DLGate, smContext.Supi)

	}

	return flowQER, nil

}

func (dpNode *DataPathNode) CreateSessRuleQer(smContext *SMContext) (*QER, error) {

	var flowQER *QER

	logger.PduSessLog.Infoln("[QER Create] Starting QER creation for node: ==supi=%s", dpNode.UPF.NodeID, smContext.Supi)

	sessionRule := smContext.SelectedSessionRule()

	if sessionRule == nil {

		logger.PduSessLog.Warnln("[QER Create] No session rule found in SMContext ==supi=%s", smContext.Supi)

		return nil, fmt.Errorf("no session rule")

	}

	logger.PduSessLog.Infof("[QER Create] SessionRule found: UL-AMBR=%s, DL-AMBR=%s ==supi=%s",

		sessionRule.AuthSessAmbr.Uplink, sessionRule.AuthSessAmbr.Downlink, smContext.Supi)

	// Get Default Qos-Data for the session

	smPolicyDec := smContext.SmPolicyUpdates[0].SmPolicyDecision

	logger.PduSessLog.Infof("[QER Create] SM Policy Decision QosData count: %d ==supi=%s", len(smPolicyDec.QosDecs), smContext.Supi)

	defQosData := qos.GetDefaultQoSDataFromPolicyDecision(smPolicyDec)

	logger.PduSessLog.Infof("[QER Create] Default QFI selected from QoS ID=%s ==supi=%s", defQosData.QosId, smContext.Supi)

	if newQER, err := dpNode.UPF.AddQER(); err != nil {

		logger.PduSessLog.Errorln("new QER failed")

		return nil, err

	} else {

		logger.PduSessLog.Infof("[QER Create] QER ID allocated: %d ==supi=%s", newQER.QERID, smContext.Supi)

		newQER.QFI.QFI = qos.GetQosFlowIdFromQosId(defQosData.QosId)

		newQER.GateStatus = &GateStatus{

			ULGate: GateOpen,

			DLGate: GateOpen,
		}

		newQER.MBR = &MBR{

			ULMBR: util.BitRateTokbps(sessionRule.AuthSessAmbr.Uplink),

			DLMBR: util.BitRateTokbps(sessionRule.AuthSessAmbr.Downlink),
		}

		logger.PduSessLog.Infof("[QER Create] Final QER setup: QFI=%d, ULMBR=%d kbps, DLMBR=%d kbps ==supi=%s", newQER.QFI.QFI, newQER.MBR.ULMBR, newQER.MBR.DLMBR, smContext.Supi)

		flowQER = newQER

	}

	logger.PduSessLog.Infoln("[QER Create] QER creation complete ==supi=%s", smContext.Supi)

	logger.PduSessLog.Infof("[QER Create] flowQER: %+v ==supi=%s", flowQER, smContext.Supi)

	return flowQER, nil

}

// CreateDedicatedQosQer creates a dedicated QER (QoS Enforcement Rule) for a PDU session in the given UPF.

// It processes the SM Policy decision for the UE and creates QERs for each dedicated QoS flow (non-default).

func (dpNode *DataPathNode) CreateDedicatedQosQer(smContext *SMContext) (*QER, error) {

	var createdQER *QER

	// Log start of QER creation

	logger.PduSessLog.Infof("CreateDedicatedQosQer: start for UE [%s], PDU Session ID [%d]",

		smContext.Supi, smContext.PDUSessionID)

	smPolicyDec := smContext.SmPolicyUpdates[0].SmPolicyDecision

	logger.PduSessLog.Infof("CreateDedicatedQosQer: total QoSData entries = %d", len(smPolicyDec.QosDecs))

	// Iterate over all QoSData in SM Policy decision

	for qosID, qosData := range smPolicyDec.QosDecs {

		// Skip default QoS flows

		if qosData.DefQosFlowIndication {

			logger.PduSessLog.Infof("CreateDedicatedQosQer: skipping default QoSData [QosId=%s]", qosID)

			continue

		}

		logger.PduSessLog.Infof("CreateDedicatedQosQer: processing dedicated QoSData [QosId=%s, 5QI=%d]", qosData.QosId, qosData.Var5qi)

		// Attempt to add a new QER in the UPF

		if newQER, err := dpNode.UPF.AddQER(); err != nil {

			// Error creating QER

			logger.PduSessLog.Errorf("CreateDedicatedQosQer: AddQER failed for UE [%s], QoSId [%s], error: %v",

				smContext.Supi, qosData.QosId, err)

			return nil, err

		} else {

			// Successfully created QER, set QFI

			newQER.QFI.QFI = qos.GetQosFlowIdFromQosId(qosData.QosId)

			// Set GateStatus: open UL and DL gates by default

			newQER.GateStatus = &GateStatus{

				ULGate: GateOpen,

				DLGate: GateOpen,
			}

			// Set Guaranteed Bit Rate (GBR) if configured

			if qosData.GbrUl != "" && qosData.GbrDl != "" {

				newQER.GBR = &GBR{

					ULGBR: util.BitRateTokbps(util.NormalizeBitRate(qosData.GbrUl)),

					DLGBR: util.BitRateTokbps(util.NormalizeBitRate(qosData.GbrDl)),
				}

				logger.PduSessLog.Infof("CreateDedicatedQosQer: GBR set [UL=%d kbps, DL=%d kbps]",

					newQER.GBR.ULGBR, newQER.GBR.DLGBR)

			} else {

				logger.PduSessLog.Infof("CreateDedicatedQosQer: no GBR configured for QoSId [%s]", qosData.QosId)

			}

			// Set Maximum Bit Rate (MBR) if configured

			if qosData.MaxbrUl != "" && qosData.MaxbrDl != "" {

				newQER.MBR = &MBR{

					ULMBR: util.BitRateTokbps(util.NormalizeBitRate(qosData.MaxbrUl)),

					DLMBR: util.BitRateTokbps(util.NormalizeBitRate(qosData.MaxbrDl)),
				}

				logger.PduSessLog.Infof("CreateDedicatedQosQer: MBR set [UL=%d kbps, DL=%d kbps]",

					newQER.MBR.ULMBR, newQER.MBR.DLMBR)

			} else {

				logger.PduSessLog.Infof("CreateDedicatedQosQer: no MBR configured for QoSId [%s]", qosData.QosId)

			}

			// Log the created QER

			logger.PduSessLog.Infof("CreateDedicatedQosQer: QER created [QER-ID=%d, QFI=%d] for UE [%s], QoSId [%s]",

				newQER.QERID, newQER.QFI.QFI, smContext.Supi, qosData.QosId)

			// Track the last created QER

			createdQER = newQER

		}

	}

	// If no dedicated QER was created, log a warning

	if createdQER == nil {

		logger.PduSessLog.Warnf("CreateDedicatedQosQer: no dedicated QER created for UE [%s]", smContext.Supi)

		return nil, nil

	}

	// Log success with last created QER

	logger.PduSessLog.Infof("CreateDedicatedQosQer: success, last created QER [QER-ID=%d, QFI=%d] for UE [%s]",

		createdQER.QERID, createdQER.QFI.QFI, smContext.Supi)

	return createdQER, nil

}

// ActivateUpLinkPdr

func (dpNode *DataPathNode) ActivateUpLinkPdr(smContext *SMContext, defQER *QER, defPrecedence uint32) error {

	logger.PduSessLog.Infof("[UL][Enter] ActivateUpLinkPdr node=%s defQER_ptr=%p ActivateUpLinkPdr Supi: ========= [%v]", dpNode.UPF.NodeID, defQER, smContext.Supi)

	logger.PduSessLog.Infof("[UL][QER] ptr=%p value=%+v ActivateUpLinkPdr Supi: ========= [%v]", defQER, defQER, smContext.Supi)

	ueIpAddr := UEIPAddress{}

	if dpNode.UPF.IsUpfSupportUeIpAddrAlloc() {

		ueIpAddr.CHV4 = true

	} else {

		ueIpAddr.V4 = true

		ueIpAddr.Ipv4Address = smContext.PDUAddress.Ip.To4()

	}

	curULTunnel := dpNode.UpLinkTunnel

	logger.PduSessLog.Info("[UL][PDR] supi=%s curULTunnel: %+v", smContext.Supi, curULTunnel)

	for name, ULPDR := range curULTunnel.PDR {

		logger.CtxLog.Infof("[UL][PDR] supi=%s BEFORE attach name=%s PDRID=%d QERs=%d precedence=%d ptr=%p", smContext.Supi, name, ULPDR.PDRID, len(ULPDR.QER), ULPDR.Precedence, ULPDR)

		prevLen := len(ULPDR.QER)

		// External lock map, keyed by PDRID

		lockI, _ := pdrLocks.LoadOrStore(ULPDR.PDRID, &sync.Mutex{})

		lock := lockI.(*sync.Mutex)

		lock.Lock()

		ULPDR.QER = append(ULPDR.QER, defQER)

		lock.Unlock()

		logger.CtxLog.Infof("[UL][PDR] supi=%s QER attach: PDRID=%d %d -> %d (addedQERID=%d)", smContext.Supi, ULPDR.PDRID, prevLen, len(ULPDR.QER),

			func() uint32 {

				if defQER != nil {

					return defQER.QERID

				}

				return 0

			}())

		// Set Default precedence

		if ULPDR.Precedence == 0 {

			logger.CtxLog.Infof("[UL][PDR] supi=%s precedence was 0, setting to default=%d (PDRID=%d)", smContext.Supi, defPrecedence, ULPDR.PDRID)

			ULPDR.Precedence = defPrecedence

		}

		ULPDR.PDI.SourceInterface = SourceInterface{InterfaceValue: SourceInterfaceAccess}

		ULPDR.PDI.LocalFTeid = &FTEID{

			Ch: true,
		}

		ULPDR.PDI.UEIPAddress = &ueIpAddr

		ULPDR.PDI.NetworkInstance = util_3gpp.Dnn(smContext.Dnn)

		ULPDR.OuterHeaderRemoval = &OuterHeaderRemoval{

			OuterHeaderRemovalDescription: OuterHeaderRemovalGtpUUdpIpv4,
		}

		ULFAR := ULPDR.FAR

		ULFAR.ApplyAction = ApplyAction{

			Buff: false,

			Drop: false,

			Dupl: false,

			Forw: true,

			Nocp: false,
		}

		ULFAR.ForwardingParameters = &ForwardingParameters{

			DestinationInterface: DestinationInterface{

				InterfaceValue: DestinationInterfaceCore,
			},

			NetworkInstance: []byte(smContext.Dnn),
		}

		if dpNode.IsAnchorUPF() {

			ULFAR.ForwardingParameters.
				DestinationInterface.InterfaceValue = DestinationInterfaceSgiLanN6Lan

		}

		if nextULDest := dpNode.Next(); nextULDest != nil {

			nextULTunnel := nextULDest.UpLinkTunnel

			iface := nextULTunnel.DestEndPoint.UPF.GetInterface(models.UpInterfaceType_N9, smContext.Dnn)

			if iface == nil {

				logger.CtxLog.Errorf(" supi=%s  UPF Interface is nil for DNN [%v]", smContext.Supi, smContext.Dnn)

				return fmt.Errorf(" supi=%s  UPF Interface is nil for DNN [%v]", smContext.Supi, smContext.Dnn)

			}

			if upIP, err := iface.IP(smContext.SelectedPDUSessionType); err != nil {

				logger.CtxLog.Errorf("supi=%s  activate UpLink PDR[%v] failed %v", smContext.Supi, name, err)

				return err

			} else {

				ULFAR.ForwardingParameters.OuterHeaderCreation = &OuterHeaderCreation{

					OuterHeaderCreationDescription: OuterHeaderCreationGtpUUdpIpv4,

					Ipv4Address: upIP,

					Teid: nextULTunnel.TEID,
				}

			}

		}

		logger.CtxLog.Infof("supi=%s  activate UpLink PDR[%v]:[%v]", smContext.Supi, name, ULPDR)

	}

	return nil

}

func (dpNode *DataPathNode) ActivateDlLinkPdr(smContext *SMContext, defQER *QER, defPrecedence uint32, dataPath *DataPath) error {

	logger.PduSessLog.Infof("[DL][Enter] ActivateDlLinkPdr node=%s defQER_ptr=%p ActivateDlLinkPdr Supi: ========= [%v]", dpNode.UPF.NodeID, defQER, smContext.Supi)

	logger.PduSessLog.Infof("[DL][QER] ptr=%p value=%+v ActivateUpLinkPdr Supi: ========= [%v]", defQER, defQER, smContext.Supi)

	var iface *UPFInterfaceInfo

	curDLTunnel := dpNode.DownLinkTunnel

	logger.PduSessLog.Infof("[DL][PDR] supi=%s curULTunnel: %+v", smContext.Supi, curDLTunnel)

	// UPF provided UE ip-addr

	ueIpAddr := UEIPAddress{}

	if dpNode.UPF.IsUpfSupportUeIpAddrAlloc() {

		ueIpAddr.CHV4 = true

	} else {

		ueIpAddr.V4 = true

		ueIpAddr.Ipv4Address = smContext.PDUAddress.Ip.To4()

	}

	for name, DLPDR := range curDLTunnel.PDR {

		logger.CtxLog.Infof("[DL][PDR] supi=%s BEFORE attach name=%s PDRID=%d QERs=%d precedence=%d ptr=%p", smContext.Supi, name, DLPDR.PDRID, len(DLPDR.QER), DLPDR.Precedence, DLPDR)

		logger.CtxLog.Infof("supi=%s activate Downlink PDR[%v]:[%v]", smContext.Supi, name, DLPDR)

		prevLen := len(DLPDR.QER)

		// External lock map, keyed by PDRID

		lockI, _ := pdrLocks.LoadOrStore(DLPDR.PDRID, &sync.Mutex{})

		lock := lockI.(*sync.Mutex)

		lock.Lock()

		DLPDR.QER = append(DLPDR.QER, defQER)

		lock.Unlock()

		logger.CtxLog.Infof("[DL][PDR] supi=%s QER attach: PDRID=%d %d -> %d (addedQERID=%d)", smContext.Supi, DLPDR.PDRID, prevLen, len(DLPDR.QER), func() uint32 {

			if defQER != nil {

				return defQER.QERID

			}

			return 0

		}())

		if DLPDR.Precedence == 0 {

			logger.CtxLog.Infof("[DL][PDR] supi=%s precedence was 0, setting to default=%d (PDRID=%d)", smContext.Supi, defPrecedence, DLPDR.PDRID)

			DLPDR.Precedence = defPrecedence

		}

		if !dpNode.IsAnchorUPF() {

			DLPDR.OuterHeaderRemoval = &OuterHeaderRemoval{

				OuterHeaderRemovalDescription: OuterHeaderRemovalGtpUUdpIpv4,
			}

		}

		DLPDR.PDI.SourceInterface = SourceInterface{InterfaceValue: SourceInterfaceCore}

		DLPDR.PDI.UEIPAddress = &ueIpAddr

		DLFAR := DLPDR.FAR

		logger.PduSessLog.Debugln(" supi=%s current DP Node IP:", smContext.Supi, dpNode.UPF.NodeID.ResolveNodeIdToIp().String())

		logger.PduSessLog.Debugln("supi= %s before DLPDR OuterHeaderCreation", smContext.Supi)

		if nextDLDest := dpNode.Prev(); nextDLDest != nil {

			logger.PduSessLog.Debugln("supi=%s ===in DLPDR OuterHeaderCreation", smContext.Supi)

			nextDLTunnel := nextDLDest.DownLinkTunnel

			DLFAR.ApplyAction = ApplyAction{

				Buff: true,

				Drop: false,

				Dupl: false,

				Forw: false,

				Nocp: true,
			}

			iface = nextDLDest.UPF.GetInterface(models.UpInterfaceType_N9, smContext.Dnn)

			if iface == nil {

				logger.CtxLog.Errorf("supi=%s  ==UPF Interface is nil for DNN [%v]", smContext.Supi, smContext.Dnn)

				return fmt.Errorf("supi=%s UPF Interface is nil for DNN [%v]", smContext.Supi, smContext.Dnn)

			}

			if upIP, err := iface.IP(smContext.SelectedPDUSessionType); err != nil {

				logger.CtxLog.Errorf("supi=%s activate Downlink PDR[%v] failed %v", smContext.Supi, name, err)

				return err

			} else {

				DLFAR.ForwardingParameters = &ForwardingParameters{

					DestinationInterface: DestinationInterface{InterfaceValue: DestinationInterfaceAccess},

					OuterHeaderCreation: &OuterHeaderCreation{

						OuterHeaderCreationDescription: OuterHeaderCreationGtpUUdpIpv4,

						Ipv4Address: upIP,

						Teid: nextDLTunnel.TEID,
					},
				}

			}

		} else {

			if anIP := smContext.Tunnel.ANInformation.IPAddress; anIP != nil {

				ANUPF := dataPath.FirstDPNode

				DefaultDLPDR := ANUPF.DownLinkTunnel.PDR["default"] // TODO: Iterate over all PDRs

				DLFAR := DefaultDLPDR.FAR

				DLFAR.ForwardingParameters = new(ForwardingParameters)

				DLFAR.ForwardingParameters.DestinationInterface.InterfaceValue = DestinationInterfaceAccess

				DLFAR.ForwardingParameters.NetworkInstance = []byte(smContext.Dnn)

				DLFAR.ForwardingParameters.OuterHeaderCreation = new(OuterHeaderCreation)

				dlOuterHeaderCreation := DLFAR.ForwardingParameters.OuterHeaderCreation

				dlOuterHeaderCreation.OuterHeaderCreationDescription = OuterHeaderCreationGtpUUdpIpv4

				dlOuterHeaderCreation.Teid = smContext.Tunnel.ANInformation.TEID

				dlOuterHeaderCreation.Ipv4Address = smContext.Tunnel.ANInformation.IPAddress.To4()

			}

		}

		logger.CtxLog.Infof("supi=%s activate Downlink PDR[%v]:[%v]", smContext.Supi, name, DLPDR)

	}

	return nil

}

// ActivateTunnelAndPDR

func (dataPath *DataPath) ActivateTunnelAndPDR(smContext *SMContext, precedence uint32) error {

	logger.PduSessLog.Info("[DP][Enter] Supi=%s ActivateTunnelAndPDR precedence=%d firstNode=%p", smContext.Supi, precedence, dataPath.FirstDPNode)

	// Check if UPF association is good

	if err := dataPath.validateDataPathUpfStatus(); err != nil {

		logger.PduSessLog.Errorln("supi=%s one or more UPF in DataPath not associated", smContext.Supi)

		return err

	}

	logger.PduSessLog.Infoln("supi=%s [DP] UPF association validated", smContext.Supi)

	// Allocate Local SEIDs

	smContext.AllocateLocalSEIDForDataPath(dataPath)

	// Allocate UL/DL PDRs for the Tunnels

	logger.PduSessLog.Infoln("supi=%s [DP] Activating UL/DL tunnels", smContext.Supi)

	if err := dataPath.ActivateUlDlTunnel(smContext); err != nil {

		logger.PduSessLog.Info("supi=%s activate UL/DL tunnel error %v", smContext.Supi, err.Error())

		return err

	}

	logger.PduSessLog.Info("[DP] UL/DL tunnels activated , supi=%s", smContext.Supi)

	// Activate PDR

	for curDataPathNode := dataPath.FirstDPNode; curDataPathNode != nil; curDataPathNode = curDataPathNode.Next() {

		logger.PduSessLog.Info("[DP][Node] node=%s UL=%t DL=%t Supi=%s", curDataPathNode.UPF.NodeID, curDataPathNode.UpLinkTunnel != nil, curDataPathNode.DownLinkTunnel != nil, smContext.Supi)

		// Add flow QER

		defQER, err := curDataPathNode.CreateSessRuleQer(smContext)

		if err != nil {

			return err

		}

		logger.PduSessLog.Info("[DP][Node %s] defQER created: ptr=%p id=%d qfi=%d supi=%s", curDataPathNode.UPF.NodeID, defQER, defQER.QERID, defQER.QFI.QFI, smContext.Supi)

		logger.CtxLog.Debugln("calculate", curDataPathNode.UPF.PFCPAddr().String())

		// Setup UpLink PDR

		if curDataPathNode.UpLinkTunnel != nil {

			logger.PduSessLog.Infof("[DP][Node %s][UL] calling ActivateUpLinkPdr with defQER ptr=%p id=%d supi=%s", curDataPathNode.UPF.NodeID, defQER, defQER.QERID, smContext.Supi)

			if err := curDataPathNode.ActivateUpLinkPdr(smContext, defQER, precedence); err != nil {

				logger.CtxLog.Errorf("supi=%s ===activate UpLink PDR error %v", smContext.Supi, err.Error())

			} else {

				// verify attach

				total := 0

				for _, p := range curDataPathNode.UpLinkTunnel.PDR {

					total += len(p.QER)

					logger.PduSessLog.Infof("[DP][Node %s][UL] PDRID=%d now has %d QER(s) supi=%s",

						curDataPathNode.UPF.NodeID, p.PDRID, len(p.QER), smContext.Supi)

				}

				logger.PduSessLog.Infof("[DP][Node %s][UL] total QERs across UL PDRs after attach=%d supi=%s", curDataPathNode.UPF.NodeID, total, smContext.Supi)

			}

		}

		// Setup DownLink PDR

		if curDataPathNode.DownLinkTunnel != nil {

			logger.PduSessLog.Infof("[DP][Node %s][DL] calling ActivateDlLinkPdr with defQER ptr=%p id=%d supi=%s", curDataPathNode.UPF.NodeID, defQER, defQER.QERID, smContext.Supi)

			if err := curDataPathNode.ActivateDlLinkPdr(smContext, defQER, precedence, dataPath); err != nil {

				logger.CtxLog.Errorf("activate DlLink PDR error %v supi=%s", err.Error(), smContext.Supi)

			} else {

				// verify attach

				total := 0

				for _, p := range curDataPathNode.DownLinkTunnel.PDR {

					total += len(p.QER)

					logger.PduSessLog.Infof("[DP][Node %s][DL] PDRID=%d now has %d QER(s) supi=%s",

						curDataPathNode.UPF.NodeID, p.PDRID, len(p.QER), smContext.Supi)

				}

				logger.PduSessLog.Infof("[DP][Node %s][DL] total QERs across DL PDRs after attach=%d supi=%s", curDataPathNode.UPF.NodeID, total, smContext.Supi)

			}

		}

		ueIpAddr := UEIPAddress{}

		if curDataPathNode.UPF.IsUpfSupportUeIpAddrAlloc() {

			ueIpAddr.CHV4 = true

		} else {

			ueIpAddr.V4 = true

			ueIpAddr.Ipv4Address = smContext.PDUAddress.Ip.To4()

		}

		if curDataPathNode.DownLinkTunnel != nil {

			if curDataPathNode.DownLinkTunnel.SrcEndPoint == nil {

				for _, DNDLPDR := range curDataPathNode.DownLinkTunnel.PDR {

					DNDLPDR.PDI.SourceInterface = SourceInterface{InterfaceValue: SourceInterfaceCore}

					DNDLPDR.PDI.NetworkInstance = util_3gpp.Dnn(smContext.Dnn)

					DNDLPDR.PDI.UEIPAddress = &ueIpAddr

					logger.PduSessLog.Infof("[DP][Node %s][DL] Filled PDI for PDRID=%d (SrcEP==nil) supi=%s", curDataPathNode.UPF.NodeID, DNDLPDR.PDRID, smContext.Supi)

				}

			}

		}

	}

	dataPath.Activated = true

	logger.PduSessLog.Infof("[DP][Exit] ActivateTunnelAndPDR completed. Activated=%v supi=%s", dataPath.Activated, smContext.Supi)

	return nil

}

func (dataPath *DataPath) DeactivateTunnelAndPDR(smContext *SMContext) {

	firstDPNode := dataPath.FirstDPNode

	// Deactivate Tunnels

	for curDataPathNode := firstDPNode; curDataPathNode != nil; curDataPathNode = curDataPathNode.Next() {

		curDataPathNode.DeactivateUpLinkTunnel(smContext)

		curDataPathNode.DeactivateDownLinkTunnel(smContext)

	}

	dataPath.Activated = false

}
