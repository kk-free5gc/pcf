package processor

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mohae/deepcopy"

	"github.com/free5gc/openapi"
	"github.com/free5gc/openapi/models"
	pcf_context "github.com/free5gc/pcf/internal/context"
	"github.com/free5gc/pcf/internal/logger"
	"github.com/free5gc/pcf/internal/util"
)

func (p *Processor) HandleDeleteUEPoliciesPolAssoId(
	c *gin.Context,
	polAssoId string,
) {
	logger.UePolicyLog.Infof("WNC: Handle UE Policy Association Delete - Policy Association ID: %s", polAssoId)

	ue := p.Context().PCFUeFindByPolicyId(polAssoId)
	if ue == nil {
		problemDetails := util.GetProblemDetail("polAssoId not found in PCF", util.CONTEXT_NOT_FOUND)
		logger.UePolicyLog.Errorf("WNC: UE Policy Association not found for polAssoId: %s", polAssoId)
		c.JSON(int(problemDetails.Status), problemDetails)
		return
	}

	if ue.UEPolicyData[polAssoId] == nil {
		problemDetails := util.GetProblemDetail("polAssoId not found in PCF", util.CONTEXT_NOT_FOUND)
		logger.UePolicyLog.Errorf("WNC: UE Policy Data not found for polAssoId: %s", polAssoId)
		c.JSON(int(problemDetails.Status), problemDetails)
		return
	}

	// Clean up the UE policy association
	delete(ue.UEPolicyData, polAssoId)
	logger.UePolicyLog.Infof("WNC: Successfully deleted UE Policy Association: %s", polAssoId)

	c.JSON(http.StatusNoContent, nil)
}

func (p *Processor) HandleGetUEPoliciesPolAssoId(
	c *gin.Context,
	polAssoId string,
) {
	logger.UePolicyLog.Infof("WNC: Handle UE Policy Association Get - Policy Association ID: %s", polAssoId)

	ue := p.Context().PCFUeFindByPolicyId(polAssoId)
	if ue == nil {
		problemDetails := util.GetProblemDetail("polAssoId not found in PCF", util.CONTEXT_NOT_FOUND)
		logger.UePolicyLog.Errorf("WNC: UE Policy Association not found for polAssoId: %s", polAssoId)
		c.JSON(int(problemDetails.Status), problemDetails)
		return
	}

	uePolicyData := ue.UEPolicyData[polAssoId]
	if uePolicyData == nil {
		problemDetails := util.GetProblemDetail("polAssoId not found in PCF", util.CONTEXT_NOT_FOUND)
		logger.UePolicyLog.Errorf("WNC: UE Policy Data not found for polAssoId: %s", polAssoId)
		c.JSON(int(problemDetails.Status), problemDetails)
		return
	}

	// Build response based on current UE policy data
	rsp := models.PcfUePolicyControlPolicyAssociation{
		SuppFeat: uePolicyData.SuppFeat,
	}

	// Add request triggers if they exist
	if uePolicyData.Triggers != nil {
		rsp.Triggers = uePolicyData.Triggers
		logger.UePolicyLog.Debugf("WNC: Returning %d triggers for polAssoId: %s", len(uePolicyData.Triggers), polAssoId)

		// According to 3GPP TS 29.525: PRAs should only be included if PRA_CH trigger is provided
		for _, trigger := range uePolicyData.Triggers {
			if trigger == models.PcfUePolicyControlRequestTrigger_PRA_CH {
				if uePolicyData.Pras != nil {
					rsp.Pras = uePolicyData.Pras
					logger.UePolicyLog.Debugf("WNC: Returning presence areas due to PRA_CH trigger for polAssoId: %s", polAssoId)
				}
				break
			}
		}
	}

	logger.UePolicyLog.Infof("WNC: Successfully retrieved UE Policy Association: %s", polAssoId)
	c.JSON(http.StatusOK, rsp)
}

func (p *Processor) HandleUpdatePostUEPoliciesPolAssoId(
	c *gin.Context,
	polAssoId string,
	policyAssociationUpdateRequest models.PcfUePolicyControlPolicyAssociationUpdateRequest,
) {
	logger.UePolicyLog.Infof("WNC: Handle UE Policy Association Update - Policy Association ID: %s", polAssoId)

	response, problemDetails := p.UpdatePostUEPoliciesPolAssoIdProcedure(polAssoId, policyAssociationUpdateRequest)
	if response != nil {
		logger.UePolicyLog.Infof("WNC: UE Policy Association update successful for polAssoId: %s", polAssoId)
		c.JSON(http.StatusOK, response)
		return
	} else if problemDetails != nil {
		logger.UePolicyLog.Errorf("WNC: UE Policy Association update failed for polAssoId: %s, error: %s", polAssoId, problemDetails.Detail)
		c.JSON(int(problemDetails.Status), problemDetails)
		return
	}

	// Default error response
	problemDetails = &models.ProblemDetails{
		Status: http.StatusForbidden,
		Cause:  "UNSPECIFIED",
	}
	logger.UePolicyLog.Errorf("WNC: UE Policy Association update failed with unspecified error for polAssoId: %s", polAssoId)
	c.JSON(int(problemDetails.Status), problemDetails)
}

func (p *Processor) UpdatePostUEPoliciesPolAssoIdProcedure(polAssoId string,
	policyAssociationUpdateRequest models.PcfUePolicyControlPolicyAssociationUpdateRequest,
) (*models.PcfUePolicyControlPolicyUpdate, *models.ProblemDetails) {
	logger.UePolicyLog.Debugf("WNC: Processing UE Policy Association update for polAssoId: %s", polAssoId)

	ue := p.Context().PCFUeFindByPolicyId(polAssoId)
	if ue == nil {
		problemDetails := util.GetProblemDetail("polAssoId not found in PCF", util.CONTEXT_NOT_FOUND)
		logger.UePolicyLog.Errorf("WNC: UE not found for polAssoId: %s", polAssoId)
		return nil, &problemDetails
	}

	uePolicyData := ue.UEPolicyData[polAssoId]
	if uePolicyData == nil {
		problemDetails := util.GetProblemDetail("polAssoId not found in PCF", util.CONTEXT_NOT_FOUND)
		logger.UePolicyLog.Errorf("WNC: UE Policy Data not found for polAssoId: %s", polAssoId)
		return nil, &problemDetails
	}

	var response models.PcfUePolicyControlPolicyUpdate

	// Update notification URI if provided
	if policyAssociationUpdateRequest.NotificationUri != "" {
		logger.UePolicyLog.Debugf("WNC: Updating notification URI for polAssoId: %s", polAssoId)
		uePolicyData.NotificationUri = policyAssociationUpdateRequest.NotificationUri
	}

	// Update alternative notification IPv4 addresses if provided
	if policyAssociationUpdateRequest.AltNotifIpv4Addrs != nil {
		logger.UePolicyLog.Debugf("WNC: Updating alternative IPv4 notification addresses for polAssoId: %s", polAssoId)
		uePolicyData.AltNotifIpv4Addrs = policyAssociationUpdateRequest.AltNotifIpv4Addrs
	}

	// Update alternative notification IPv6 addresses if provided
	if policyAssociationUpdateRequest.AltNotifIpv6Addrs != nil {
		logger.UePolicyLog.Debugf("WNC: Updating alternative IPv6 notification addresses for polAssoId: %s", polAssoId)
		uePolicyData.AltNotifIpv6Addrs = policyAssociationUpdateRequest.AltNotifIpv6Addrs
	}

	// Process triggers - similar to AM Policy but for UE-specific triggers
	for _, trigger := range policyAssociationUpdateRequest.Triggers {
		logger.UePolicyLog.Debugf("WNC: Processing trigger: %s for polAssoId: %s", trigger, polAssoId)

		switch trigger {
		case models.PcfUePolicyControlRequestTrigger_LOC_CH:
			// Location change trigger
			if policyAssociationUpdateRequest.UserLoc == nil {
				problemDetail := util.GetProblemDetail("UserLoc is nil", util.ERROR_REQUEST_PARAMETERS)
				logger.UePolicyLog.Warnf("WNC: UserLoc missing for LOC_CH trigger, polAssoId: %s", polAssoId)
				return nil, &problemDetail
			}
			uePolicyData.UserLoc = policyAssociationUpdateRequest.UserLoc
			logger.UePolicyLog.Infof("WNC: Updated UE[%s] UserLocation %+v for polAssoId: %s", ue.Supi, uePolicyData.UserLoc, polAssoId)

		case models.PcfUePolicyControlRequestTrigger_PLMN_CH:
			// PLMN change trigger
			if policyAssociationUpdateRequest.PlmnId == nil {
				problemDetail := util.GetProblemDetail("PlmnId is nil", util.ERROR_REQUEST_PARAMETERS)
				logger.UePolicyLog.Warnf("WNC: PlmnId missing for PLMN_CH trigger, polAssoId: %s", polAssoId)
				return nil, &problemDetail
			}
			uePolicyData.ServingPlmn = policyAssociationUpdateRequest.PlmnId
			logger.UePolicyLog.Infof("WNC: Updated serving PLMN for UE[%s], polAssoId: %s", ue.Supi, polAssoId)

		case models.PcfUePolicyControlRequestTrigger_CON_STATE_CH:
			// Connectivity state change trigger
			if policyAssociationUpdateRequest.ConnectState == "" {
				problemDetail := util.GetProblemDetail("ConnectState is nil", util.ERROR_REQUEST_PARAMETERS)
				logger.UePolicyLog.Warnf("WNC: ConnectState missing for CON_STATE_CH trigger, polAssoId: %s", polAssoId)
				return nil, &problemDetail
			}
			// Store connectivity state in UE context (note: there's no direct field for this in UeUEPolicyData)
			logger.UePolicyLog.Infof("WNC: Updated UE connectivity state to %s for UE[%s], polAssoId: %s", policyAssociationUpdateRequest.ConnectState, ue.Supi, polAssoId)

		case models.PcfUePolicyControlRequestTrigger_PRA_CH:
			// Presence Reporting Area change trigger
			if policyAssociationUpdateRequest.PraStatuses == nil {
				problemDetail := util.GetProblemDetail("PraStatuses are nil", util.ERROR_REQUEST_PARAMETERS)
				logger.UePolicyLog.Warnf("WNC: PraStatuses missing for PRA_CH trigger, polAssoId: %s", polAssoId)
				return nil, &problemDetail
			}
			for praId, praInfo := range policyAssociationUpdateRequest.PraStatuses {
				logger.UePolicyLog.Infof("WNC: Presence Area Id[%s] changed state to %s for polAssoId: %s", praId, praInfo.PresenceState, polAssoId)
			}
			uePolicyData.Pras = policyAssociationUpdateRequest.PraStatuses

		default:
			logger.UePolicyLog.Warnf("WNC: Unsupported trigger type: %s for polAssoId: %s", trigger, polAssoId)
		}
	}

	// Set response triggers to current policy data triggers
	response.Triggers = uePolicyData.Triggers

	// TODO: Add more sophisticated policy update logic here
	// For now, we maintain basic association management

	logger.UePolicyLog.Infof("WNC: UE Policy Association update completed successfully for polAssoId: %s", polAssoId)
	return &response, nil
}

func (p *Processor) HandlePostUEPolicies(
	c *gin.Context,
	polAssoId string,
	policyAssociationRequest models.PcfUePolicyControlPolicyAssociationRequest,
) {
	logger.UePolicyLog.Infof("WNC: Handle UE Policy Create Request")

	response, locationHeader, problemDetails := p.PostUEPoliciesProcedure(polAssoId, policyAssociationRequest)
	if response != nil {
		c.Header("Location", locationHeader)
		c.JSON(http.StatusCreated, response)

		// Extract association ID from location header for async policy evaluation
		// Location header format: /npcf-ue-policy-control/v1/policies/{actualAssociationId}
		actualAssociationId := ""
		if locationHeader != "" {
			if idx := strings.LastIndex(locationHeader, "/"); idx != -1 {
				actualAssociationId = locationHeader[idx+1:]
				logger.UePolicyLog.Tracef("WNC: UEPolicy CREATE actualAssociationId: %s", actualAssociationId)
			}
		}

		if actualAssociationId != "" {
			// Trigger async UE policy evaluation after successful association creation
			// Following 3GPP TS 23.502 Clause 4.2.4.3 transparent policy delivery
			go p.TriggerAsyncUEPolicyEvaluation(policyAssociationRequest.Supi, actualAssociationId)
		} else {
			logger.UePolicyLog.Errorf("WNC: Failed to extract association ID from location header: %s", locationHeader)
		}

		return
	} else if problemDetails != nil {
		c.JSON(int(problemDetails.Status), problemDetails)
		return
	}

	problemDetails = &models.ProblemDetails{
		Status: http.StatusForbidden,
		Cause:  "UNSPECIFIED",
	}
	c.JSON(int(problemDetails.Status), problemDetails)
}

func (p *Processor) PostUEPoliciesProcedure(polAssoId string,
	policyAssociationRequest models.PcfUePolicyControlPolicyAssociationRequest,
) (*models.PcfUePolicyControlPolicyAssociation, string, *models.ProblemDetails) {
	var response models.PcfUePolicyControlPolicyAssociation
	pcfSelf := p.Context()
	var ue *pcf_context.UeContext
	if val, ok := pcfSelf.UePool.Load(policyAssociationRequest.Supi); ok {
		ue = val.(*pcf_context.UeContext)
	}
	if ue == nil {
		if newUe, err := pcfSelf.NewPCFUe(policyAssociationRequest.Supi); err != nil {
			// supi format dose not match "imsi-..."
			problemDetail := util.GetProblemDetail("Supi Format Error", util.ERROR_REQUEST_PARAMETERS)
			logger.UePolicyLog.Errorln("WNC:", err.Error())
			return nil, "", &problemDetail
		} else {
			ue = newUe
		}
	}
	udrUri := p.getUdrUri(ue)
	if udrUri == "" {
		// Can't find any UDR support this Ue
		pcfSelf.UePool.Delete(ue.Supi)
		problemDetail := util.GetProblemDetail("Ue is not supported in PCF", util.USER_UNKNOWN)
		logger.UePolicyLog.Errorf("WNC: Ue[%s] is not supported in PCF", ue.Supi)
		return nil, "", &problemDetail
	}
	ue.UdrUri = udrUri

	response.Request = deepcopy.Copy(&policyAssociationRequest).(*models.PcfUePolicyControlPolicyAssociationRequest)
	assolId := fmt.Sprintf("%s-ue-policy-%d", ue.Supi, ue.UePolicyAssociationIDGenerator)
	uePolicy := ue.UEPolicyData[assolId]

	if uePolicy == nil {
		// TODO: Get UE Policy Data from UDR (similar to AM policy)
		// For now, create basic association without actual UE policies
		// Future implementation should call p.Consumer().GetUEPolicyData(ue)

		uePolicy = ue.NewUeUEPolicyData(assolId, policyAssociationRequest)
	}

	// TODO: according to PCF Policy to determine SuppFeat, Triggers, etc.
	var requestSuppFeat openapi.SupportedFeature
	if suppFeat, err := openapi.NewSupportedFeature(policyAssociationRequest.SuppFeat); err != nil {
		logger.UePolicyLog.Warnln("WNC:", err)
	} else {
		requestSuppFeat = suppFeat
	}

	// Negotiate supported features with UE Policy Control service
	if pcfSelf.PcfSuppFeats[models.ServiceName_NPCF_UE_POLICY_CONTROL] != nil {
		uePolicy.SuppFeat = pcfSelf.PcfSuppFeats[models.ServiceName_NPCF_UE_POLICY_CONTROL].NegotiateWith(requestSuppFeat).String()
	} else {
		uePolicy.SuppFeat = policyAssociationRequest.SuppFeat
	}
	response.SuppFeat = uePolicy.SuppFeat

	// TODO: add Reports
	// response.Triggers
	// response.Pras
	ue.UePolicyAssociationIDGenerator++
	// Create location header for update, delete, get
	locationHeader := util.GetResourceUri(models.ServiceName_NPCF_UE_POLICY_CONTROL, assolId)
	logger.UePolicyLog.Tracef("WNC: UEPolicy association Id[%s] Create", assolId)

	// if consumer is AMF then subscribe this AMF Status
	if policyAssociationRequest.Guami != nil {
		// if policyAssociationRequest.Guami has been subscribed, then no need to subscribe again
		needSubscribe := true
		pcfSelf.AMFStatusSubsData.Range(func(key, value interface{}) bool {
			data := value.(pcf_context.AMFStatusSubscriptionData)
			for _, guami := range data.GuamiList {
				if reflect.DeepEqual(guami, *policyAssociationRequest.Guami) {
					needSubscribe = false
					break
				}
			}
			// if no need to subscribe => stop iteration
			return needSubscribe
		})

		if needSubscribe {
			logger.UePolicyLog.Debugf("WNC: Subscribe AMF status change[GUAMI: %+v]", *policyAssociationRequest.Guami)
			amfUri := p.Consumer().SendNFInstancesAMF(pcfSelf.NrfUri,
				*policyAssociationRequest.Guami, models.ServiceName_NAMF_COMM)
			if amfUri != "" {
				problemDetails, err := p.Consumer().AmfStatusChangeSubscribe(amfUri,
					[]models.Guami{*policyAssociationRequest.Guami})
				if err != nil {
					logger.UePolicyLog.Errorf("WNC: Subscribe AMF status change error[%+v]", err)
				} else if problemDetails != nil {
					logger.UePolicyLog.Errorf("WNC: Subscribe AMF status change failed[%+v]", problemDetails)
				} else {
					logger.UePolicyLog.Debugf("WNC: AMF status subscription successful for GUAMI: %+v", *policyAssociationRequest.Guami)
				}
			}
		} else {
			logger.UePolicyLog.Debugf("WNC: AMF status[GUAMI: %+v] has been subscribed", *policyAssociationRequest.Guami)
		}
	}
	return &response, locationHeader, nil
}

// TriggerAsyncUEPolicyEvaluation evaluates whether UE policy update is needed
// and triggers transparent policy delivery via AMF if required
// Following 3GPP TS 23.502 Clause 4.2.4.3
func (p *Processor) TriggerAsyncUEPolicyEvaluation(supi string, polAssoId string) {
	logger.UePolicyLog.Infof("WNC: Starting async UE policy evaluation for SUPI: %s, polAssoId: %s", supi, polAssoId)

	pcfSelf := p.Context()
	ue, exists := pcfSelf.UePool.Load(supi)
	if !exists {
		logger.UePolicyLog.Errorf("WNC: UE context not found for SUPI: %s", supi)
		return
	}

	ueContext := ue.(*pcf_context.UeContext)
	uePolicyData := ueContext.UEPolicyData[polAssoId]
	if uePolicyData == nil {
		logger.UePolicyLog.Errorf("WNC: UE policy data not found for polAssoId: %s", polAssoId)
		return
	}

	// TODO: Implement PSI (Policy Set Information) comparison logic
	// For now, always trigger policy update on initial registration
	shouldUpdatePolicy := true

	if shouldUpdatePolicy {
		logger.UePolicyLog.Infof("WNC: Policy update required for SUPI: %s, initiating transparent delivery", supi)

		// Trigger transparent UE policy delivery via AMF
		err := p.UpdateUEPolicy(ueContext, polAssoId)
		if err != nil {
			logger.UePolicyLog.Errorf("WNC: Failed to update UE policy for SUPI: %s, error: %v", supi, err)
		} else {
			logger.UePolicyLog.Infof("WNC: UE policy update completed successfully for SUPI: %s", supi)
		}
	} else {
		logger.UePolicyLog.Infof("WNC: No policy update needed for SUPI: %s", supi)
	}
}

// UpdateUEPolicy implements transparent UE policy delivery via AMF N1N2MessageTransfer
// Following 3GPP TS 23.502 Clause 4.2.4.3 transparent policy delivery procedure
func (p *Processor) UpdateUEPolicy(ue *pcf_context.UeContext, polAssoId string) error {
	logger.UePolicyLog.Infof("WNC: Starting UE policy update for SUPI: %s, polAssoId: %s", ue.Supi, polAssoId)

	uePolicyData := ue.UEPolicyData[polAssoId]
	if uePolicyData == nil {
		return fmt.Errorf("UE policy data not found for polAssoId: %s", polAssoId)
	}

	// Get AMF URI from the UE policy association notification URI
	amfUri := ""
	if uePolicyData.NotificationUri != "" {
		// Extract AMF base URI from notification URI
		// NotificationUri format: http://127.0.0.10:8000/namf-callback/v1/ue-policy/
		notifUri := uePolicyData.NotificationUri
		// Extract base URI (everything before /namf-callback)
		if idx := strings.Index(notifUri, "/namf-callback"); idx != -1 {
			amfUri = notifUri[:idx]
		}
	}

	if amfUri == "" {
		return fmt.Errorf("cannot determine AMF URI from notification URI")
	}

	logger.UePolicyLog.Debugf("WNC: Using AMF URI: %s for SUPI: %s", amfUri, ue.Supi)

	// Build UE policy container payload
	policyPayload, err := p.buildUEPolicyContainer(ue, polAssoId)
	if err != nil {
		return fmt.Errorf("failed to build UE policy container: %v", err)
	}

	// Build N1N2MessageTransferRequest for UE policy delivery
	n1n2Request := models.N1N2MessageTransferRequest{
		JsonData: &models.N1N2MessageTransferReqData{
			N1MessageContainer: &models.N1MessageContainer{
				N1MessageClass:   models.N1MessageClass_UPDP, // UE Policy Delivery and Processing
				N1MessageContent: &models.RefToBinaryData{ContentId: "n1SmMsg"},
			},
		},
		BinaryDataN1Message: policyPayload,
	}

	// Get token context for authentication
	ctx, pd, err := p.Context().GetTokenCtx(models.ServiceName_NAMF_COMM, models.NrfNfManagementNfType_AMF)
	if err != nil {
		logger.UePolicyLog.Errorf("WNC: Failed to get token context: %v", err)
		return err
	}
	if pd != nil {
		logger.UePolicyLog.Errorf("WNC: Problem getting token context: %+v", pd)
		return fmt.Errorf("authentication failed")
	}

	// Call AMF N1N2MessageTransfer for transparent policy delivery
	response, err := p.Consumer().N1N2MessageTransfer(ctx, ue.Supi, n1n2Request, amfUri)
	if err != nil {
		logger.UePolicyLog.Errorf("WNC: N1N2MessageTransfer failed for SUPI: %s, error: %v", ue.Supi, err)
		return err
	}

	if response != nil {
		logger.UePolicyLog.Infof("WNC: N1N2MessageTransfer response - Cause: %s for SUPI: %s",
			response.Cause, ue.Supi)

		// Update UE policy association state based on response
		switch response.Cause {
		case models.N1N2MessageTransferCause_N1_N2_TRANSFER_INITIATED:
			logger.UePolicyLog.Infof("WNC: Policy delivery initiated for SUPI: %s", ue.Supi)
			// Update UDR with PSI list after successful delivery
			err = p.updateUDRPolicySetInformation(ue, polAssoId)
			if err != nil {
				logger.UePolicyLog.Warnf("WNC: Failed to update UDR PSI for SUPI: %s, error: %v", ue.Supi, err)
			}
		case models.N1N2MessageTransferCause_ATTEMPTING_TO_REACH_UE:
			logger.UePolicyLog.Infof("WNC: Attempting to reach UE via paging for SUPI: %s", ue.Supi)
			// UDR update will be done after UE response is received
		default:
			logger.UePolicyLog.Warnf("WNC: Unexpected transfer cause: %s for SUPI: %s", response.Cause, ue.Supi)
		}
	}

	logger.UePolicyLog.Infof("WNC: UE policy update request completed for SUPI: %s", ue.Supi)
	return nil
}

// buildUEPolicyContainer constructs the UE policy container payload for transparent delivery
// Uses the same policy loading and building logic as AMF for consistency
func (p *Processor) buildUEPolicyContainer(ue *pcf_context.UeContext, polAssoId string) ([]byte, error) {
	logger.UePolicyLog.Debugf("WNC: Building UE policy container for SUPI: %s, polAssoId: %s", ue.Supi, polAssoId)

	// TODO: For now, we'll return a basic policy container
	// In a full implementation, this should:
	// 1. Load policy from UDR or configured sources
	// 2. Build proper URSP rules and traffic descriptors
	// 3. Support policy fragmentation for large payloads
	// 4. Include PSI (Policy Set Information) for tracking

	// For initial implementation, construct a minimal but valid policy container
	// Following 3GPP TS 24.501 UE policy container format

	var payload []byte

	// Procedure Transaction Identity (PTI) - using 1 for policy delivery
	payload = append(payload, 0x01)

	// Message Type: MANAGE UE POLICY COMMAND (0x01)
	payload = append(payload, 0x01)

	// UE policy section management list length (2 bytes, big-endian)
	// For minimal implementation, we'll have an empty policy list
	payload = append(payload, 0x00, 0x00) // 0 length policy list

	logger.UePolicyLog.Debugf("WNC: Built minimal UE policy container with %d bytes for SUPI: %s",
		len(payload), ue.Supi)

	// TODO: Future enhancement - integrate with existing AMF policy building logic:
	// - Load policy from configured YAML files
	// - Use BuildManageUEPolicyCommand() from AMF policy package
	// - Support complex URSP rules and traffic descriptors
	// - Implement proper policy versioning and PSI tracking

	return payload, nil
}

// updateUDRPolicySetInformation updates PSI list in UDR after successful policy delivery
func (p *Processor) updateUDRPolicySetInformation(ue *pcf_context.UeContext, polAssoId string) error {
	logger.UePolicyLog.Debugf("WNC: Updating UDR PSI for SUPI: %s, polAssoId: %s", ue.Supi, polAssoId)

	// TODO: Build proper PSI list based on delivered policy
	// For now, create a basic PSI entry
	psiList := []string{
		"policy-set-1",
		// TODO: Add proper policy set identifiers
		// - URSP rule IDs
		// - Policy version identifiers
		// - Delivery timestamps
		// - Policy section references
	}

	problemDetails, err := p.Consumer().UpdateUEPolicySectionIdentifier(ue, psiList)
	if err != nil {
		logger.UePolicyLog.Errorf("WNC: UDR PSI update failed for SUPI: %s, error: %v", ue.Supi, err)
		return err
	}

	if problemDetails != nil {
		logger.UePolicyLog.Errorf("WNC: UDR PSI update problem for SUPI: %s, details: %+v", ue.Supi, problemDetails)
		return fmt.Errorf("UDR update failed: %s", problemDetails.Detail)
	}

	logger.UePolicyLog.Infof("WNC: UDR PSI update completed successfully for SUPI: %s", ue.Supi)
	return nil
}
