package consumer

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/free5gc/openapi"
	"github.com/free5gc/openapi/amf/Communication"
	"github.com/free5gc/openapi/models"
	pcf_context "github.com/free5gc/pcf/internal/context"
	"github.com/free5gc/pcf/internal/logger"
	"github.com/free5gc/pcf/pkg/factory"
)

type namfService struct {
	consumer *Consumer

	nfComMu sync.RWMutex

	nfComClients map[string]*Communication.APIClient
}

func (s *namfService) getNFCommunicationClient(uri string) *Communication.APIClient {
	if uri == "" {
		return nil
	}
	s.nfComMu.RLock()
	client, ok := s.nfComClients[uri]
	if ok {
		defer s.nfComMu.RUnlock()
		return client
	}

	configuration := Communication.NewConfiguration()
	configuration.SetBasePath(uri)
	client = Communication.NewAPIClient(configuration)

	s.nfComMu.RUnlock()
	s.nfComMu.Lock()
	defer s.nfComMu.Unlock()
	s.nfComClients[uri] = client
	return client
}

func (s *namfService) AmfStatusChangeSubscribe(amfUri string, guamiList []models.Guami) (
	problemDetails *models.ProblemDetails, err error,
) {
	logger.ConsumerLog.Debugf("PCF Subscribe to AMF status[%+v]", amfUri)
	pcfContext := s.consumer.pcf.Context()

	// Set client and set url
	client := s.getNFCommunicationClient(amfUri)

	subscriptionData := models.AmfCommunicationSubscriptionData{
		AmfStatusUri: fmt.Sprintf("%s"+factory.PcfCallbackResUriPrefix+"/amfstatus", pcfContext.GetIPv4Uri()),
		GuamiList:    guamiList,
	}
	amfStausChangeRequest := &Communication.AMFStatusChangeSubscribeRequest{}
	amfStausChangeRequest.SetAmfCommunicationSubscriptionData(subscriptionData)
	ctx, pd, err := pcfContext.GetTokenCtx(models.ServiceName_NAMF_COMM, models.NrfNfManagementNfType_AMF)
	if err != nil {
		return pd, err
	}
	res, localErr := client.SubscriptionsCollectionCollectionApi.AMFStatusChangeSubscribe(
		ctx, amfStausChangeRequest)

	if localErr == nil {
		locationHeader := res.Location
		logger.ConsumerLog.Debugf("location header: %+v", locationHeader)

		subscriptionID := locationHeader[strings.LastIndex(locationHeader, "/")+1:]
		amfStatusSubsData := pcf_context.AMFStatusSubscriptionData{
			AmfUri:       amfUri,
			AmfStatusUri: res.AmfCommunicationSubscriptionData.AmfStatusUri,
			GuamiList:    res.AmfCommunicationSubscriptionData.GuamiList,
		}
		pcfContext.NewAmfStatusSubscription(subscriptionID, amfStatusSubsData)
	}

	if genericErr, ok := localErr.(openapi.GenericOpenAPIError); ok {
		if problemDetails, ok := genericErr.Model().(models.ProblemDetails); ok {
			return &problemDetails, nil
		}

		logger.ConsumerLog.Errorf("openapi error: %+v", localErr)
		return nil, localErr
	}
	return nil, localErr
}

// N1N2MessageTransfer sends UE policy container to AMF for transparent delivery to UE
// Following the same pattern as SMF's N1N2MessageTransfer implementation
func (s *namfService) N1N2MessageTransfer(
	ctx context.Context, supi string, n1n2Request models.N1N2MessageTransferRequest, apiPrefix string,
) (*models.N1N2MessageTransferRspData, error) {
	logger.ConsumerLog.Infof("WNC: PCF initiating N1N2MessageTransfer to AMF for SUPI: %s", supi)
	
	client := s.getNFCommunicationClient(apiPrefix)
	if client == nil {
		logger.ConsumerLog.Errorf("WNC: N1N2MessageTransfer client is nil for AMF URI: %s", apiPrefix)
		return nil, fmt.Errorf("N1N2MessageTransfer client is nil: (%v)", apiPrefix)
	}

	n1n2MessageTransferRequest := &Communication.N1N2MessageTransferRequest{
		UeContextId:                &supi,
		N1N2MessageTransferRequest: &n1n2Request,
	}

	logger.ConsumerLog.Debugf("WNC: Sending N1N2MessageTransfer request to AMF: %s", apiPrefix)
	
	rsp, err := client.N1N2MessageCollectionCollectionApi.N1N2MessageTransfer(ctx, n1n2MessageTransferRequest)
	if err != nil {
		logger.ConsumerLog.Errorf("WNC: N1N2MessageTransfer failed: %v", err)
		return nil, err
	}
	
	if rsp == nil {
		logger.ConsumerLog.Error("WNC: N1N2MessageTransfer response is nil")
		return nil, fmt.Errorf("N1N2MessageTransfer response is nil")
	}

	logger.ConsumerLog.Infof("WNC: N1N2MessageTransfer completed successfully for SUPI: %s", supi)
	return &rsp.N1N2MessageTransferRspData, nil
}
