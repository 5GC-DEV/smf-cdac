// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"fmt"
	"net/http"

	"github.com/5GC-DEV/nas-cdac"
	"github.com/5GC-DEV/openapi-cdac/Nsmf_PDUSession"
	"github.com/5GC-DEV/openapi-cdac/models"
	"github.com/5GC-DEV/util-cdac/httpwrapper"
	"github.com/omec-project/smf/consumer"
	"github.com/omec-project/smf/context"
	smf_context "github.com/omec-project/smf/context"
	"github.com/omec-project/smf/metrics"
	"github.com/omec-project/smf/msgtypes/svcmsgtypes"
	"github.com/omec-project/smf/transaction"
)

type pfcpAction struct {
	sendPfcpModify, sendPfcpDelete bool
}

type pfcpParam struct {
	pdrList   []*context.PDR
	farList   []*context.FAR
	barList   []*context.BAR
	qerList   []*context.QER
	removePDR []*context.PDR // Add for teardown
	removeFAR []*smf_context.FAR
	removeQER []*smf_context.QER
}

func HandleUpdateN1Msg(txn *transaction.Transaction, response *models.UpdateSmContextResponse, pfcpAction *pfcpAction, pfcpParam *pfcpParam) error {
	body := txn.Req.(models.UpdateSmContextRequest)
	smContext := txn.Ctxt.(*context.SMContext)
	tunnel := smContext.Tunnel

	if body.BinaryDataN1SmMessage != nil {
		smContext.SubPduSessLog.Debugln("PDUSessionSMContextUpdate, Binary Data N1 SmMessage isn't nil")
		m := nas.NewMessage()
		err := m.GsmMessageDecode(&body.BinaryDataN1SmMessage)
		smContext.SubPduSessLog.Debugln("PDUSessionSMContextUpdate, Update SM Context Request N1SmMessage:", m)
		if err != nil {
			smContext.SubPduSessLog.Error(err)
			txn.Rsp = &httpwrapper.Response{
				Status: http.StatusForbidden,
				Body: models.UpdateSmContextErrorResponse{
					JsonData: &models.SmContextUpdateError{
						Error: &Nsmf_PDUSession.N1SmError,
					},
				}, // Depends on the reason why N4 fail
			}
			return err
		}
		switch m.GsmHeader.GetMessageType() {
		case nas.MsgTypePDUSessionReleaseRequest:
			smContext.SubPduSessLog.Infof("PDUSessionSMContextUpdate, N1 Msg PDU Session Release Request received")
			pduSessIDRelReq := int32(m.PDUSessionReleaseRequest.GetPDUSessionID())
			smContext.SubPduSessLog.Debug("PDU Session ID in Rel Req: ", pduSessIDRelReq)
			pduSessIDSmCxt := smContext.PDUSessionID
			smContext.SubPduSessLog.Debug("PDU Session ID in SM Context: ", pduSessIDSmCxt)
			if smContext.SMContextState != context.SmStateActive {
				// Wait till the state becomes SmStateActive again
				// TODO: implement sleep wait in concurrent architecture
				smContext.SubPduSessLog.Infof("PDUSessionSMContextUpdate, SM Context State[%v] should be SmStateActive", smContext.SMContextState.String())
			}
			if pduSessIDRelReq == pduSessIDSmCxt {
				smContext.HandlePDUSessionReleaseRequest(m.PDUSessionReleaseRequest)
				metrics.IncrementSessReleaseStats(smf_context.SMF_Self().NfInstanceID, string(svcmsgtypes.NsmfPDUSessionRelease), "Out", "success")
				if buf, err := context.BuildGSMPDUSessionReleaseCommand(smContext); err != nil {
					smContext.SubPduSessLog.Errorf("PDUSessionSMContextUpdate, build GSM PDUSessionReleaseCommand failed: %+v", err)
				} else {
					response.BinaryDataN1SmMessage = buf
				}

				response.JsonData.N1SmMsg = &models.RefToBinaryData{ContentId: "PDUSessionReleaseCommand"}

				response.JsonData.N2SmInfo = &models.RefToBinaryData{ContentId: "PDUResourceReleaseCommand"}
				response.JsonData.N2SmInfoType = models.N2SmInfoType_PDU_RES_REL_CMD

				if buf, err := context.BuildPDUSessionResourceReleaseCommandTransfer(smContext); err != nil {
					smContext.SubPduSessLog.Errorf("PDUSessionSMContextUpdate, build PDUSessionResourceReleaseCommandTransfer failed: %+v", err)
				} else {
					response.BinaryDataN2SmInformation = buf
				}

				if smContext.Tunnel != nil {
					smContext.ChangeState(context.SmStatePfcpModify)
					smContext.SubCtxLog.Debugln("PDUSessionSMContextUpdate, SMContextState Change State:", smContext.SMContextState.String())
					// Send release to UPF
					// releaseTunnel(smContext)
					pfcpAction.sendPfcpDelete = true
				} else {
					smContext.ChangeState(context.SmStateModify)
					smContext.SubCtxLog.Debugln("PDUSessionSMContextUpdate, SMContextState Change State:", smContext.SMContextState.String())
				}
				err := smContext.ReleasePduSessionID()
				if err != nil {
					smContext.SubGsmLog.Infof("release PDU Session ID failed: %s", err)
				}
			} else {
				smContext.SubPduSessLog.Errorf("Invalid PDU Session ID")
				if buf, err := context.BuildGSMPDUSessionReleaseRejectWithCause(smContext, pduSessIDRelReq, "InvalidPDUSessionIdentity"); err != nil {
					smContext.SubPduSessLog.Errorf("PDUSessionSMContextRelease, build GSM PDUSessionReleaseReject failed: %+v", err)
				} else {
					response.BinaryDataN1SmMessage = buf
				}
				metrics.IncrementSessReleaseStats(smf_context.SMF_Self().NfInstanceID, string(svcmsgtypes.NsmfPDUSessionRelease), "Out", "failure")
				response.JsonData.N1SmMsg = &models.RefToBinaryData{ContentId: "PDUSessionReleaseReject"}
				smContext.ChangeState(context.SmStateModify)
				// smContext.SubCtxLog.Debugln("PDUSessionSMContextUpdate, SMContextState Change State:", smContext.SMContextState.String())
				smContext.SubCtxLog.Infoln("PDUSessionSMContextUpdate, SMContextState Change State:", smContext.SMContextState.String())
			}
		case nas.MsgTypePDUSessionReleaseComplete:
			smContext.SubPduSessLog.Infoln("PDUSessionSMContextUpdate, N1 Msg PDU Session Release Complete received")
			if smContext.SMContextState != context.SmStateInActivePending {
				// Wait till the state becomes SmStateActive again
				// TODO: implement sleep wait in concurrent architecture
				smContext.SubPduSessLog.Infof("PDUSessionSMContextUpdate, SMContext State[%v] should be SmStateInActivePending State", smContext.SMContextState.String())
			}
			// Send Release Notify to AMF
			smContext.SubPduSessLog.Infof("PDUSessionSMContextUpdate, send Update SmContext Response")
			smContext.ChangeState(context.SmStateInit)
			smContext.SubCtxLog.Debugln("PDUSessionSMContextUpdate, SMContextState Change State:", smContext.SMContextState.String())
			response.JsonData.UpCnxState = models.UpCnxState_DEACTIVATED
			/*problemDetails, err := consumer.SendSMContextStatusNotification(smContext.SmStatusNotifyUri)
			if problemDetails != nil || err != nil {
				if problemDetails != nil {
					smContext.SubPduSessLog.Warnf("PDUSessionSMContextUpdate, send SMContext Status Notification Problem[%+v]", problemDetails)
				}

				if err != nil {
					smContext.SubPduSessLog.Warnf("PDUSessionSMContextUpdate, send SMContext Status Notification Error[%v]", err)
				}
			} else {
				smContext.SubPduSessLog.Debugln("PDUSessionSMContextUpdate, sent SMContext Status Notification successfully")
			}*/
			smContext.SubPduSessLog.Debugln("PDUSessionSMContextUpdate, sent SMContext Status Notification successfully")
		case nas.MsgTypePDUSessionEstablishmentRequest:
			smContext.SubPduSessLog.Debugf("PDUSessionSMContextUpdate, N1 Msg PDU Session Establishment Request received")
			if smContext.SMContextState != context.SmStateInActivePending {
				// Wait till the state becomes SmStateActive again
				// TODO: implement sleep wait in concurrent architecture
				smContext.SubPduSessLog.Debugf("PDUSessionSMContextUpdate, SMContext State[%v] should be SmStateInActivePending State", smContext.SMContextState.String())
			}

			smContext := txn.Ctxt.(*smf_context.SMContext)

			pduSessIDRelReq := int32(m.PDUSessionEstablishmentRequest.GetPDUSessionID())
			smContext.SubPduSessLog.Debugf("PDU Session ID in Rel Req: ", pduSessIDRelReq)
			pduSessIDSmCxt := smContext.PDUSessionID
			smContext.SubPduSessLog.Debugf("PDU Session ID in SM Context: ", pduSessIDSmCxt)
			if pduSessIDRelReq == pduSessIDSmCxt && pduSessIDSmCxt != 0 {
				if smContext.SMContextState != context.SmStateActive {
					// Wait till the state becomes Active again
					// TODO: implement sleep wait in concurrent architecture
					smContext.SubPduSessLog.Warnf("PDUSessionSMContextUpdate, SMContext state[%v] should be Active",
						smContext.SMContextState.String())
				}

				if smContext.PDUSessionID == 0 {
					smContext.SubPduSessLog.Debugf("Context PDU Session ID is 0. Updating Context to Request ID: %d", pduSessIDRelReq)
					smContext.PDUSessionID = pduSessIDRelReq
				}

				smContext.ChangeState(context.SmStateModify)
				smContext.SubCtxLog.Debugf("PDUSessionSMContextUpdate, SMContextState Change State:", smContext.SMContextState.String())
				pdrList := []*context.PDR{}
				farList := []*context.FAR{}

				smContext.PendingUPF = make(context.PendingUPF)
				for _, dataPath := range tunnel.DataPathPool {
					if dataPath.Activated {
						ANUPF := dataPath.FirstDPNode
						for _, DLPDR := range ANUPF.DownLinkTunnel.PDR {
							DLPDR.FAR.ApplyAction = context.ApplyAction{Buff: false, Drop: false, Dupl: false, Forw: true, Nocp: false}
							DLPDR.FAR.ForwardingParameters = &context.ForwardingParameters{
								DestinationInterface: context.DestinationInterface{
									InterfaceValue: context.DestinationInterfaceAccess,
								},
								NetworkInstance: []byte(smContext.Dnn),
							}

							DLPDR.State = context.RULE_UPDATE
							DLPDR.FAR.State = context.RULE_UPDATE

							pdrList = append(pdrList, DLPDR)
							farList = append(farList, DLPDR.FAR)

							if _, exist := smContext.PendingUPF[ANUPF.GetNodeIP()]; !exist {
								smContext.SubPfcpLog.Debugf(
									"[PFCP] Adding UPF to PendingUPF UE[%s] UPF[%s]",
									smContext.Supi,
									ANUPF.GetNodeIP(),
								)
								smContext.PendingUPF[ANUPF.GetNodeIP()] = true
							}
						}
					}
				}

				pfcpParam.pdrList = append(pfcpParam.pdrList, pdrList...)
				pfcpParam.farList = append(pfcpParam.farList, farList...)

				pfcpAction.sendPfcpModify = true
				smContext.ChangeState(context.SmStatePfcpModify)
				smContext.SubCtxLog.Debugf("PDUSessionSMContextUpdate, SMContextState Change State:", smContext.SMContextState.String())
				if err := SendPfcpSessionModifyReq(smContext, pfcpParam); err != nil {
					// PFCP modify failed — revert state and return error
					smContext.SubCtxLog.Errorf("PFCP session modify error: %v", err)
					// smContext.ChangeState(prevState)
					smContext.SubCtxLog.Debugf("SMContext[%s-%02d] state reverted to %s after PFCP error",
						smContext.Supi, smContext.PDUSessionID, smContext.SMContextState.String())

					// Build HTTP error response for the original transaction
					httpResponse := makePduCtxtModifyErrRsp(smContext, err.Error())
					txn.Err = err
					txn.Rsp = httpResponse
					return err
				}
				// Set response and change state to active
				smContext.ChangeState(smf_context.SmStateActive)
				smContext.SubCtxLog.Debugf("PFCP Modify success and N1N2 Msg sent, new state:", smContext.SMContextState.String())

				httpResponse := &httpwrapper.Response{
					Status: http.StatusCreated,
					Body:   nil,
				}
				txn.Rsp = httpResponse

				return nil
			} else {
				if smContext.PDUSessionID == 0 {
					smContext.SubPduSessLog.Infof("Context PDU Session ID is 0. Updating Context to Request ID: %d", pduSessIDRelReq)
					smContext.PDUSessionID = pduSessIDRelReq
				}
				smContext.SubPduSessLog.Debugf("Invalid PDU Session ID")
				txn.Rsp = smContext.GeneratePDUSessionEstablishmentReject("PDUSessionDoesNotExist")
				return fmt.Errorf("PDUSessionDoesNotExist")
			}
		}
	} else {
		smContext.SubPduSessLog.Debugln("PDUSessionSMContextUpdate, Binary Data N1 SmMessage is nil")
	}

	return nil
}

func HandleUpCnxState(txn *transaction.Transaction, response *models.UpdateSmContextResponse, pfcpAction *pfcpAction, pfcpParam *pfcpParam) error {
	body := txn.Req.(models.UpdateSmContextRequest)
	smContext := txn.Ctxt.(*context.SMContext)
	smContextUpdateData := body.JsonData

	switch smContextUpdateData.UpCnxState {
	case models.UpCnxState_ACTIVATING:
		smContext.SubPduSessLog.Infof("PDUSessionSMContextUpdate, UP cnx state %v received", smContextUpdateData.UpCnxState)
		if smContext.SMContextState != context.SmStateActive {
			// Wait till the state becomes SmStateActive again
			// TODO: implement sleep wait in concurrent architecture
			smContext.SubPduSessLog.Infof("PDUSessionSMContextUpdate, SMContext State[%v] should be SmStateActive State", smContext.SMContextState.String())
		}
		smContext.ChangeState(context.SmStateModify)
		smContext.SubCtxLog.Debugln("PDUSessionSMContextUpdate, SMContextState Change State:", smContext.SMContextState.String())
		response.JsonData.N2SmInfo = &models.RefToBinaryData{ContentId: "PDUSessionResourceSetupRequestTransfer"}
		response.JsonData.UpCnxState = models.UpCnxState_ACTIVATING
		response.JsonData.N2SmInfoType = models.N2SmInfoType_PDU_RES_SETUP_REQ

		n2Buf, err := context.BuildPDUSessionResourceSetupRequestTransfer(smContext)
		if err != nil {
			smContext.SubPduSessLog.Errorf("PDUSessionSMContextUpdate, build PDUSession Resource Setup Request Transfer Error(%s)", err.Error())
		}
		smContext.UpCnxState = models.UpCnxState_ACTIVATING
		response.BinaryDataN2SmInformation = n2Buf
		response.JsonData.N2SmInfoType = models.N2SmInfoType_PDU_RES_SETUP_REQ
	case models.UpCnxState_DEACTIVATED:
		smContext.SubPduSessLog.Infof("PDUSessionSMContextUpdate, UP cnx state %v received", smContextUpdateData.UpCnxState)
		if smContext.SMContextState != context.SmStateActive {
			// Wait till the state becomes Active again
			// TODO: implement sleep wait in concurrent architecture
			smContext.SubPduSessLog.Infof("PDUSessionSMContextUpdate, SMContext State[%v] should be Active State", smContext.SMContextState.String())
		}
		if smContext.Tunnel != nil {
			smContext.ChangeState(context.SmStateModify)
			smContext.SubCtxLog.Debugln("PDUSessionSMContextUpdate, SMContextState Change State:", smContext.SMContextState.String())
			response.JsonData.UpCnxState = models.UpCnxState_DEACTIVATED
			smContext.UpCnxState = body.JsonData.UpCnxState
			smContext.UeLocation = body.JsonData.UeLocation
			// TODO: Deactivate N2 downlink tunnel
			// Set FAR and An, N3 Release Info
			farList := []*context.FAR{}
			smContext.PendingUPF = make(context.PendingUPF)
			for _, dataPath := range smContext.Tunnel.DataPathPool {
				ANUPF := dataPath.FirstDPNode
				for _, DLPDR := range ANUPF.DownLinkTunnel.PDR {
					if DLPDR == nil {
						smContext.SubPduSessLog.Errorf("AN Release Error")
					} else {
						DLPDR.FAR.State = context.RULE_UPDATE
						DLPDR.FAR.ApplyAction.Forw = false
						DLPDR.FAR.ApplyAction.Buff = true
						DLPDR.FAR.ApplyAction.Nocp = true
						// Set DL Tunnel info to nil
						if DLPDR.FAR.ForwardingParameters != nil {
							DLPDR.FAR.ForwardingParameters.OuterHeaderCreation = nil
						}
						smContext.SubPfcpLog.Debugf(
							"[PFCP] Adding UPF to PendingUPF UE[%s] UPF[%s]",
							smContext.Supi,
							ANUPF.GetNodeIP(),
						)
						smContext.PendingUPF[ANUPF.GetNodeIP()] = true
						farList = append(farList, DLPDR.FAR)
					}
				}
			}

			pfcpParam.farList = append(pfcpParam.farList, farList...)

			pfcpAction.sendPfcpModify = true
			smContext.ChangeState(context.SmStatePfcpModify)
			smContext.SubCtxLog.Debugln("PDUSessionSMContextUpdate, SMContextState Change State:", smContext.SMContextState.String())
		}
	}
	return nil
}

func HandleUpdateHoState(txn *transaction.Transaction, response *models.UpdateSmContextResponse, pfcpAction *pfcpAction, pfcpParam *pfcpParam) error {
	body := txn.Req.(models.UpdateSmContextRequest)
	smContext := txn.Ctxt.(*context.SMContext)
	smContextUpdateData := body.JsonData
	tunnel := smContext.Tunnel

	switch smContextUpdateData.HoState {
	case models.HoState_PREPARING:
		smContext.SubPduSessLog.Infof("PDUSessionSMContextUpdate, Ho state %v received", smContextUpdateData.HoState)
		smContext.SubPduSessLog.Debugln("PDUSessionSMContextUpdate, in HoState_PREPARING")
		if smContext.SMContextState != context.SmStateActive {
			// Wait till the state becomes SmStateActive again
			// TODO: implement sleep wait in concurrent architecture
			smContext.SubPduSessLog.Warnf("PDUSessionSMContextUpdate, SMContext state[%v] should be SmStateActive",
				smContext.SMContextState.String())
		}
		smContext.ChangeState(context.SmStateModify)
		smContext.SubCtxLog.Debugln("PDUSessionSMContextUpdate, SMContextState Change State:", smContext.SMContextState.String())
		smContext.HoState = models.HoState_PREPARING
		if err := context.HandleHandoverRequiredTransfer(body.BinaryDataN2SmInformation, smContext); err != nil {
			smContext.SubPduSessLog.Errorf("PDUSessionSMContextUpdate, handle HandoverRequiredTransfer failed: %+v", err)
		}
		response.JsonData.N2SmInfoType = models.N2SmInfoType_PDU_RES_SETUP_REQ

		if n2Buf, err := context.BuildPDUSessionResourceSetupRequestTransfer(smContext); err != nil {
			smContext.SubPduSessLog.Errorf("PDUSessionSMContextUpdate, build PDUSession Resource Setup Request Transfer Error(%s)", err.Error())
		} else {
			response.BinaryDataN2SmInformation = n2Buf
		}
		response.JsonData.N2SmInfoType = models.N2SmInfoType_PDU_RES_SETUP_REQ
		response.JsonData.N2SmInfo = &models.RefToBinaryData{
			ContentId: "PDU_RES_SETUP_REQ",
		}
		response.JsonData.HoState = models.HoState_PREPARING
	case models.HoState_PREPARED:
		smContext.SubPduSessLog.Infof("PDUSessionSMContextUpdate, Ho state %v received", smContextUpdateData.HoState)
		smContext.SubPduSessLog.Debugln("PDUSessionSMContextUpdate, in HoState_PREPARED")
		if smContext.SMContextState != context.SmStateActive {
			// Wait till the state becomes SmStateActive again
			// TODO: implement sleep wait in concurrent architecture
			smContext.SubPduSessLog.Warnf("PDUSessionSMContextUpdate, SMContext state [%v] should be SmStateActive",
				smContext.SMContextState.String())
		}
		smContext.ChangeState(context.SmStateModify)
		smContext.SubCtxLog.Debugln("PDUSessionSMContextUpdate, SMContextState Change State:", smContext.SMContextState.String())
		smContext.HoState = models.HoState_PREPARED
		response.JsonData.HoState = models.HoState_PREPARED
		if err := context.HandleHandoverRequestAcknowledgeTransfer(body.BinaryDataN2SmInformation, smContext); err != nil {
			smContext.SubPduSessLog.Errorf("PDUSessionSMContextUpdate, handle HandoverRequestAcknowledgeTransfer failed: %+v", err)
		}

		if n2Buf, err := context.BuildHandoverCommandTransfer(smContext); err != nil {
			smContext.SubPduSessLog.Errorf("PDUSessionSMContextUpdate, build PDUSession Resource Setup Request Transfer Error(%s)", err.Error())
		} else {
			response.BinaryDataN2SmInformation = n2Buf
		}

		response.JsonData.N2SmInfoType = models.N2SmInfoType_HANDOVER_CMD
		response.JsonData.N2SmInfo = &models.RefToBinaryData{
			ContentId: "HANDOVER_CMD",
		}
		response.JsonData.HoState = models.HoState_PREPARING
	case models.HoState_COMPLETED:
		smContext.SubPduSessLog.Infof("PDUSessionSMContextUpdate, Ho state %v received", smContextUpdateData.HoState)
		smContext.SubPduSessLog.Debugln("PDUSessionSMContextUpdate, in HoState_COMPLETED")

		if smContext.SMContextState != context.SmStateActive {
			err := fmt.Errorf("invalid SMContext state %v for HoState_COMPLETED",
				smContext.SMContextState.String())
			smContext.SubPduSessLog.Error(err)
			return err
		}

		if tunnel == nil {
			err := fmt.Errorf("tunnel is nil during HoState_COMPLETED")
			smContext.SubPduSessLog.Error(err)
			return err
		}

		if tunnel.DataPathPool == nil {
			err := fmt.Errorf("DataPathPool is nil during HoState_COMPLETED")
			smContext.SubPduSessLog.Error(err)
			return err
		}

		pdrList := []*context.PDR{}
		farList := []*context.FAR{}

		smContext.PendingUPF = make(context.PendingUPF)

		for _, dataPath := range tunnel.DataPathPool {
			if dataPath == nil {
				continue
			}

			if dataPath.Activated {
				ANUPF := dataPath.FirstDPNode
				if ANUPF == nil || ANUPF.DownLinkTunnel == nil {
					continue
				}

				for _, DLPDR := range ANUPF.DownLinkTunnel.PDR {
					if DLPDR == nil || DLPDR.FAR == nil {
						continue
					}

					DLPDR.FAR.ApplyAction = context.ApplyAction{
						Buff: false, Drop: false, Dupl: false, Forw: true, Nocp: false,
					}

					DLPDR.State = context.RULE_UPDATE
					DLPDR.FAR.State = context.RULE_UPDATE

					pdrList = append(pdrList, DLPDR)
					farList = append(farList, DLPDR.FAR)

					if _, exist := smContext.PendingUPF[ANUPF.GetNodeIP()]; !exist {
						smContext.SubPfcpLog.Debugf(
							"[PFCP] Adding UPF to PendingUPF UE[%s] UPF[%s]",
							smContext.Supi,
							ANUPF.GetNodeIP(),
						)
						smContext.PendingUPF[ANUPF.GetNodeIP()] = true
					}
				}
			}
		}

		pfcpParam.pdrList = append(pfcpParam.pdrList, pdrList...)
		pfcpParam.farList = append(pfcpParam.farList, farList...)

		pfcpAction.sendPfcpModify = true
		smContext.ChangeState(context.SmStatePfcpModify)

		smContext.SubCtxLog.Infoln(
			"PDUSessionSMContextUpdate, SMContextState Change State:",
			smContext.SMContextState.String(),
		)

		smContext.HoState = models.HoState_COMPLETED
		response.JsonData.HoState = models.HoState_COMPLETED
	}
	return nil
}

func HandleUpdateCause(txn *transaction.Transaction, response *models.UpdateSmContextResponse, pfcpAction *pfcpAction) error {
	body := txn.Req.(models.UpdateSmContextRequest)
	smContext := txn.Ctxt.(*context.SMContext)
	smContextUpdateData := body.JsonData

	switch smContextUpdateData.Cause {
	case models.Cause_REL_DUE_TO_DUPLICATE_SESSION_ID:
		smContext.SubPduSessLog.Infof("PDUSessionSMContextUpdate, update cause %v received", smContextUpdateData.Cause)
		//* release PDU Session Here
		if smContext.SMContextState != context.SmStateActive {
			// Wait till the state becomes SmStateActive again
			// TODO: implement sleep wait in concurrent architecture
			smContext.SubPduSessLog.Warnf("PDUSessionSMContextUpdate, SMContext state[%v] should be SmStateActive",
				smContext.SMContextState.String())
		}

		response.JsonData.N2SmInfo = &models.RefToBinaryData{ContentId: "PDUResourceReleaseCommand"}
		response.JsonData.N2SmInfoType = models.N2SmInfoType_PDU_RES_REL_CMD
		smContext.PDUSessionRelease_DUE_TO_DUP_PDU_ID = true

		buf, err := context.BuildPDUSessionResourceReleaseCommandTransfer(smContext)
		response.BinaryDataN2SmInformation = buf
		if err != nil {
			smContext.SubPduSessLog.Error(err)
		}

		smContext.SubCtxLog.Infof("PDUSessionSMContextUpdate, Cause_REL_DUE_TO_DUPLICATE_SESSION_ID")

		smContext.ChangeState(context.SmStatePfcpModify)
		smContext.SubCtxLog.Debugln("PDUSessionSMContextUpdate, SMContextState Change State:", smContext.SMContextState.String())

		// releaseTunnel(smContext)
		pfcpAction.sendPfcpDelete = true
	}

	return nil
}

func HandleUpdateN2Msg(txn *transaction.Transaction, response *models.UpdateSmContextResponse, pfcpAction *pfcpAction, pfcpParam *pfcpParam) error {
	body := txn.Req.(models.UpdateSmContextRequest)
	smContext := txn.Ctxt.(*context.SMContext)
	smContextUpdateData := body.JsonData
	if smContext.Tunnel == nil {
		smContext.SubPduSessLog.Errorf("No tunnel present for SMContext %s during modify", smContext.Ref)
		return nil
	}
	tunnel := smContext.Tunnel
	switch smContextUpdateData.N2SmInfoType {
	case models.N2SmInfoType_PDU_RES_SETUP_RSP:
		smContext.SubPduSessLog.Infof("PDUSessionSMContextUpdate, N2 SM info type %v received",
			smContextUpdateData.N2SmInfoType)
		if smContext.SMContextState != context.SmStateActive {
			// Wait till the state becomes Active again
			// TODO: implement sleep wait in concurrent architecture
			smContext.SubPduSessLog.Warnf("PDUSessionSMContextUpdate, SMContext state[%v] should be Active",
				smContext.SMContextState.String())
		}
		smContext.ChangeState(context.SmStateModify)
		smContext.SubCtxLog.Debugln("PDUSessionSMContextUpdate, SMContextState Change State:", smContext.SMContextState.String())
		pdrList := []*context.PDR{}
		farList := []*context.FAR{}

		smContext.PendingUPF = make(context.PendingUPF)
		if tunnel.DataPathPool == nil {
			smContext.SubPduSessLog.Errorf("Tunnel DataPathPool is nil for %s", smContext.Ref)
			return nil
		}
		for _, dataPath := range tunnel.DataPathPool {
			if dataPath.Activated {
				ANUPF := dataPath.FirstDPNode
				for _, DLPDR := range ANUPF.DownLinkTunnel.PDR {
					DLPDR.FAR.ApplyAction = context.ApplyAction{Buff: false, Drop: false, Dupl: false, Forw: true, Nocp: false}
					DLPDR.FAR.ForwardingParameters = &context.ForwardingParameters{
						DestinationInterface: context.DestinationInterface{
							InterfaceValue: context.DestinationInterfaceAccess,
						},
						NetworkInstance: []byte(smContext.Dnn),
					}

					DLPDR.State = context.RULE_UPDATE
					DLPDR.FAR.State = context.RULE_UPDATE

					pdrList = append(pdrList, DLPDR)
					farList = append(farList, DLPDR.FAR)

					if _, exist := smContext.PendingUPF[ANUPF.GetNodeIP()]; !exist {
						smContext.SubPfcpLog.Debugf(
							"[PFCP] Adding UPF to PendingUPF UE[%s] UPF[%s]",
							smContext.Supi,
							ANUPF.GetNodeIP(),
						)
						smContext.PendingUPF[ANUPF.GetNodeIP()] = true
					}
				}
			}
		}

		if err := context.
			HandlePDUSessionResourceSetupResponseTransfer(body.BinaryDataN2SmInformation, smContext); err != nil {
			smContext.SubPduSessLog.Errorf("PDUSessionSMContextUpdate, handle PDUSessionResourceSetupResponseTransfer failed: %+v", err)
		}

		pfcpParam.pdrList = append(pfcpParam.pdrList, pdrList...)
		pfcpParam.farList = append(pfcpParam.farList, farList...)

		pfcpAction.sendPfcpModify = true
		smContext.ChangeState(context.SmStatePfcpModify)
		smContext.SubCtxLog.Debugln("PDUSessionSMContextUpdate, SMContextState Change State:", smContext.SMContextState.String())
	case models.N2SmInfoType_PDU_RES_SETUP_FAIL:
		smContext.SubPduSessLog.Infof("PDUSessionSMContextUpdate, N2 SM info type %v received",
			smContextUpdateData.N2SmInfoType)
		if err := context.
			HandlePDUSessionResourceSetupResponseTransfer(body.BinaryDataN2SmInformation, smContext); err != nil {
			smContext.SubPduSessLog.Errorf("PDUSessionSMContextUpdate, handle PDUSessionResourceSetupResponseTransfer failed: %+v", err)
		}
	case models.N2SmInfoType_PDU_RES_REL_RSP:
		smContext.SubPduSessLog.Infof("PDUSessionSMContextUpdate, N2 SM info type %v received",
			smContextUpdateData.N2SmInfoType)
		smContext.SubPduSessLog.Infof("PDUSessionSMContextUpdate, N2 PDUSession Release Complete ")
		if smContext.PDUSessionRelease_DUE_TO_DUP_PDU_ID {
			if smContext.SMContextState != context.SmStateInActivePending {
				// Wait till the state becomes Active again
				// TODO: implement sleep wait in concurrent architecture
				smContext.SubPduSessLog.Warnf("PDUSessionSMContextUpdate, SMContext state[%v] should be ActivePending",
					smContext.SMContextState.String())
			}
			smContext.ChangeState(context.SmStateInit)
			smContext.SubCtxLog.Debugln("PDUSessionSMContextUpdate, SMContextState Change State:", smContext.SMContextState.String())
			smContext.SubPduSessLog.Infof("PDUSessionSMContextUpdate, send Update SmContext Response")
			response.JsonData.UpCnxState = models.UpCnxState_DEACTIVATED

			smContext.PDUSessionRelease_DUE_TO_DUP_PDU_ID = false
			context.RemoveSMContext(smContext.Ref)
			problemDetails, err := consumer.SendSMContextStatusNotification(smContext.SmStatusNotifyUri)
			if problemDetails != nil || err != nil {
				if problemDetails != nil {
					smContext.SubPduSessLog.Warnf("PDUSessionSMContextUpdate, send SMContext Status Notification Problem[%+v]", problemDetails)
				}

				if err != nil {
					smContext.SubPduSessLog.Warnf("PDUSessionSMContextUpdate, send SMContext Status Notification Error[%v]", err)
				}
			} else {
				smContext.SubPduSessLog.Debugln("PDUSessionSMContextUpdate, send SMContext Status Notification successfully")
			}
		} else { // normal case
			if smContext.SMContextState != context.SmStateInActivePending {
				// Wait till the state becomes Active again
				// TODO: implement sleep wait in concurrent architecture
				smContext.SubPduSessLog.Warnf("PDUSessionSMContextUpdate, SMContext state[%v] should be ActivePending",
					smContext.SMContextState.String())
			}
			smContext.SubPduSessLog.Infof("PDUSessionSMContextUpdate, send Update SmContext Response")
			smContext.ChangeState(context.SmStateInActivePending)
			smContext.SubCtxLog.Debugln("PDUSessionSMContextUpdate, SMContextState Change State:", smContext.SMContextState.String())
		}
	case models.N2SmInfoType_PATH_SWITCH_REQ:
		smContext.SubPduSessLog.Infof("PDUSessionSMContextUpdate, N2 SM info type %v received",
			smContextUpdateData.N2SmInfoType)
		smContext.SubPduSessLog.Debugln("PDUSessionSMContextUpdate, handle Path Switch Request")
		if smContext.SMContextState != context.SmStateActive {
			// Wait till the state becomes Active again
			// TODO: implement sleep wait in concurrent architecture
			smContext.SubPduSessLog.Warnf("PDUSessionSMContextUpdate, SMContext state[%v] should be Active",
				smContext.SMContextState.String())
		}
		smContext.ChangeState(context.SmStateModify)
		smContext.SubCtxLog.Debugln("PDUSessionSMContextUpdate, SMContextState Change State:", smContext.SMContextState.String())

		if err := context.HandlePathSwitchRequestTransfer(body.BinaryDataN2SmInformation, smContext); err != nil {
			smContext.SubPduSessLog.Errorf("PDUSessionSMContextUpdate, handle PathSwitchRequestTransfer: %+v", err)
		}

		if n2Buf, err := context.BuildPathSwitchRequestAcknowledgeTransfer(smContext); err != nil {
			smContext.SubPduSessLog.Errorf("PDUSessionSMContextUpdate, build Path Switch Transfer Error(%+v)", err)
		} else {
			response.BinaryDataN2SmInformation = n2Buf
		}

		response.JsonData.N2SmInfoType = models.N2SmInfoType_PATH_SWITCH_REQ_ACK
		response.JsonData.N2SmInfo = &models.RefToBinaryData{
			ContentId: "PATH_SWITCH_REQ_ACK",
		}

		pdrList := []*context.PDR{}
		farList := []*context.FAR{}
		smContext.PendingUPF = make(context.PendingUPF)
		if tunnel.DataPathPool == nil {
			smContext.SubPduSessLog.Errorf("Tunnel DataPathPool is nil for %s", smContext.Ref)
			return nil
		}
		for _, dataPath := range tunnel.DataPathPool {
			if dataPath.Activated {
				ANUPF := dataPath.FirstDPNode
				for _, DLPDR := range ANUPF.DownLinkTunnel.PDR {
					pdrList = append(pdrList, DLPDR)
					farList = append(farList, DLPDR.FAR)

					if _, exist := smContext.PendingUPF[ANUPF.GetNodeIP()]; !exist {
						smContext.SubPfcpLog.Debugf(
							"[PFCP] Adding UPF to PendingUPF UE[%s] UPF[%s]",
							smContext.Supi,
							ANUPF.GetNodeIP(),
						)
						smContext.PendingUPF[ANUPF.GetNodeIP()] = true
					}
				}
			}
		}

		pfcpParam.pdrList = append(pfcpParam.pdrList, pdrList...)
		pfcpParam.farList = append(pfcpParam.farList, farList...)

		pfcpAction.sendPfcpModify = true
		smContext.ChangeState(context.SmStatePfcpModify)
		smContext.SubCtxLog.Debugln("PDUSessionSMContextUpdate, SMContextState Change State:", smContext.SMContextState.String())
	case models.N2SmInfoType_PATH_SWITCH_SETUP_FAIL:
		smContext.SubPduSessLog.Infof("PDUSessionSMContextUpdate, N2 SM info type %v received",
			smContextUpdateData.N2SmInfoType)
		if smContext.SMContextState != context.SmStateActive {
			// Wait till the state becomes SmStateActive again
			// TODO: implement sleep wait in concurrent architecture
			smContext.SubPduSessLog.Warnf("PDUSessionSMContextUpdate, SMContext state[%v] should be SmStateActive",
				smContext.SMContextState.String())
		}
		smContext.ChangeState(context.SmStateModify)
		smContext.SubCtxLog.Debugln("PDUSessionSMContextUpdate, SMContextState Change State:", smContext.SMContextState.String())
		if err := context.HandlePathSwitchRequestSetupFailedTransfer(body.BinaryDataN2SmInformation, smContext); err != nil {
			smContext.SubPduSessLog.Error()
		}
	case models.N2SmInfoType_HANDOVER_REQUIRED:
		smContext.SubPduSessLog.Infof("PDUSessionSMContextUpdate, N2 SM info type %v received",
			smContextUpdateData.N2SmInfoType)
		if smContext.SMContextState != context.SmStateActive {
			// Wait till the state becomes SmStateActive again
			// TODO: implement sleep wait in concurrent architecture
			smContext.SubPduSessLog.Warnf("PDUSessionSMContextUpdate, SMContext state[%v] should be SmStateActive",
				smContext.SMContextState.String())
		}
		smContext.ChangeState(context.SmStateModify)
		smContext.SubCtxLog.Debugln("PDUSessionSMContextUpdate, SMContextState Change State:", smContext.SMContextState.String())
		response.JsonData.N2SmInfo = &models.RefToBinaryData{ContentId: "Handover"}
	}

	return nil
}
