package policy

// DUPLICATION NOTICE: this file is a byte-for-byte port of the AMF UE-policy encoder at
// NFs/amf/internal/policy/ue_policy.go. It lives in the PCF module because Go forbids
// importing another module's internal package, so the two copies must be kept in sync by
// hand — any change to the config structs, the YAML parsing in convertYAMLToInternalConfig,
// or the byte encoding in BuildManageUEPolicyCommand (and its helpers) must be mirrored in
// both files. The encoding is guarded in both packages by TestBuildManageUEPolicyCommand_TMobile.

import (
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/free5gc/pcf/internal/logger"
)

// UE Policy Configuration Structures (Legacy format for internal use)
type UEPolicyConfig struct {
	PLMN             PLMNConfig          `json:"plmn"`
	Instructions     []InstructionConfig `json:"instructions"`
	ProcedureTransID uint8               `json:"procedureTransId"`
}

type PLMNConfig struct {
	MCC string `json:"mcc"`
	MNC string `json:"mnc"`
}

type InstructionConfig struct {
	UPSC       uint16             `json:"upsc"`
	PolicyPart UEPolicyPartConfig `json:"policyPart"`
}

type UEPolicyPartConfig struct {
	Type      uint8      `json:"type"` // 1 for URSP
	URSPRules []URSPRule `json:"urspRules"`
}

type URSPRule struct {
	Precedence          uint8                      `json:"precedence"`
	TrafficDescriptors  []TrafficDescriptor        `json:"trafficDescriptors"`
	RouteSelectionDescs []RouteSelectionDescriptor `json:"routeSelectionDescs"`
}

type TrafficDescriptor struct {
	Type        uint8  `json:"type"` // 1=match-all, 8=OS Id + OS App Id, 136=DNN, 16=IPv4 remote address
	DNN         string `json:"dnn,omitempty"`
	IPv4Address string `json:"ipv4Address,omitempty"`
	IPv4Mask    string `json:"ipv4Mask,omitempty"`
	OSId        []byte `json:"osId,omitempty"`    // 16-byte OS Id (from UUID) for type 8
	OSAppId     string `json:"osAppId,omitempty"` // OS App Id string for type 8
}

type RouteSelectionDescriptor struct {
	Precedence uint8                     `json:"precedence"`
	Components []RouteSelectionComponent `json:"components"`
}

type RouteSelectionComponent struct {
	Type                uint8   `json:"type"` // 2=S-NSSAI, 4=DNN, 8=PDU session type, 16=Preferred access type
	SNSSAI              *SNSSAI `json:"snssai,omitempty"`
	DNN                 string  `json:"dnn,omitempty"`
	PDUSessionType      uint8   `json:"pduSessionType,omitempty"`
	PreferredAccessType uint8   `json:"preferredAccessType,omitempty"`
}

type SNSSAI struct {
	SST uint8  `json:"sst"`
	SD  uint32 `json:"sd"`
}

// YAML Configuration Structures
type YAMLUEPolicyConfig struct {
	PolicySectionManagementList []YAMLPolicySectionManagement `yaml:"policySectionManagementList"`
}

type YAMLPolicySectionManagement struct {
	PlmnId       YAMLPlmnIdConfig        `yaml:"plmnId"`
	Instructions []YAMLInstructionConfig `yaml:"instructions"`
}

type YAMLPlmnIdConfig struct {
	MCC string `yaml:"mcc"`
	MNC string `yaml:"mnc"`
}

type YAMLInstructionConfig struct {
	UPSC        uint16                 `yaml:"upsc"`
	PolicyParts []YAMLPolicyPartConfig `yaml:"policyParts"`
}

type YAMLPolicyPartConfig struct {
	Type       string         `yaml:"type"`
	URSPRules  []YAMLURSPRule `yaml:"urspRules,omitempty"`
	ANDSPRules []interface{}  `yaml:"andspRules,omitempty"`
}

type YAMLURSPRule struct {
	Precedence                uint8                            `yaml:"precedence"`
	TrafficDescriptor         []YAMLTrafficDescriptorComponent `yaml:"trafficDescriptor"`
	RouteSelectionDescriptors []YAMLRouteSelectionDescriptor   `yaml:"routeSelectionDescriptors"`
}

type YAMLTrafficDescriptorComponent struct {
	Type    string `yaml:"type"`
	Value   string `yaml:"value,omitempty"`
	OSId    string `yaml:"osId,omitempty"`    // UUID string, e.g. "97a498e3-fc92-5c94-8986-0333d06e4e47"
	OSAppId string `yaml:"osAppId,omitempty"` // OS App Id string, e.g. "PRIORITIZE_LATENCY"
}

type YAMLRouteSelectionDescriptor struct {
	Precedence uint8                               `yaml:"precedence"`
	Components []YAMLRouteSelectionComponentConfig `yaml:"components"`
}

type YAMLRouteSelectionComponentConfig struct {
	Type  string `yaml:"type"`
	Value string `yaml:"value,omitempty"`
}

// QXDMPolicyConfig is a minimal match-all URSP used as a fallback when the
// configured YAML cannot be loaded.
var QXDMPolicyConfig = UEPolicyConfig{
	PLMN: PLMNConfig{MCC: "001", MNC: "01"},
	Instructions: []InstructionConfig{
		{
			UPSC: 0,
			PolicyPart: UEPolicyPartConfig{
				Type: 1, // URSP
				URSPRules: []URSPRule{
					{
						Precedence: 255,
						TrafficDescriptors: []TrafficDescriptor{
							{Type: 1}, // Match-all
						},
						RouteSelectionDescs: []RouteSelectionDescriptor{
							{
								Precedence: 255,
								Components: []RouteSelectionComponent{
									{Type: 2, SNSSAI: &SNSSAI{SST: 1, SD: 1}},
									{Type: 4, DNN: "internet"},
									{Type: 16, PreferredAccessType: 1},
								},
							},
						},
					},
				},
			},
		},
	},
	ProcedureTransID: 128,
}

// LoadUEPolicyConfigFromYAML loads UE policy configuration from YAML file
func LoadUEPolicyConfigFromYAML(configPath string) (*UEPolicyConfig, error) {
	logger.UePolicyLog.Infof("WNC: Loading UE Policy config from: %s", configPath)

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("WNC: UE Policy config file not found: %s", configPath)
	}

	// Read YAML file
	yamlFile, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("WNC: Failed to read UE Policy config file: %v", err)
	}

	// Parse YAML
	var yamlConfig YAMLUEPolicyConfig
	if err := yaml.Unmarshal(yamlFile, &yamlConfig); err != nil {
		return nil, fmt.Errorf("WNC: Failed to parse UE Policy config YAML: %v", err)
	}

	// Convert YAML config to internal structures
	internalConfig, err := convertYAMLToInternalConfig(&yamlConfig)
	if err != nil {
		return nil, fmt.Errorf("WNC: Failed to convert YAML config: %v", err)
	}

	logger.UePolicyLog.Infof("WNC: Successfully loaded UE Policy config with %d PLMN sublists",
		len(yamlConfig.PolicySectionManagementList))
	return internalConfig, nil
}

// Convert YAML config to internal structures for encoding
func convertYAMLToInternalConfig(yamlConfig *YAMLUEPolicyConfig) (*UEPolicyConfig, error) {
	if len(yamlConfig.PolicySectionManagementList) == 0 {
		return nil, fmt.Errorf("WNC: No policy section management list found in config")
	}

	// For now, use the first PLMN sublist
	plmnSublist := yamlConfig.PolicySectionManagementList[0]

	internalConfig := &UEPolicyConfig{
		PLMN: PLMNConfig{
			MCC: plmnSublist.PlmnId.MCC,
			MNC: plmnSublist.PlmnId.MNC,
		},
		Instructions:     make([]InstructionConfig, 0),
		ProcedureTransID: 128, // Default value
	}

	// Convert instructions
	for _, instruction := range plmnSublist.Instructions {
		internalInstruction := InstructionConfig{
			UPSC: instruction.UPSC,
		}
		// WNC: an instruction with no policyParts deletes the UE policy section named by
		// its UPSC (TS 24.501 Annex D). PolicyPart.Type stays 0 (a reserved part type) so
		// buildInstruction encodes the UPSC only, with empty UE policy section contents.
		if len(instruction.PolicyParts) > 0 {
			// URSP
			internalInstruction.PolicyPart.Type = 1
		}

		// Convert policy parts
		for _, policyPart := range instruction.PolicyParts {
			if policyPart.Type == "URSP" {
				for _, urspRule := range policyPart.URSPRules {
					internalRule := URSPRule{
						Precedence:          urspRule.Precedence,
						TrafficDescriptors:  make([]TrafficDescriptor, 0),
						RouteSelectionDescs: make([]RouteSelectionDescriptor, 0),
					}

					// Convert traffic descriptors
					for _, trafficDesc := range urspRule.TrafficDescriptor {
						internalTrafficDesc := TrafficDescriptor{}

						switch trafficDesc.Type {
						case "MatchAll":
							internalTrafficDesc.Type = 0x01
						case "DNN":
							internalTrafficDesc.Type = 0x88
							internalTrafficDesc.DNN = trafficDesc.Value
						case "IPv4RemoteAddress":
							internalTrafficDesc.Type = 0x10
							// Parse IPv4 address (assuming format like "192.168.1.1/255.255.255.0")
							if strings.Contains(trafficDesc.Value, "/") {
								parts := strings.Split(trafficDesc.Value, "/")
								if len(parts) == 2 {
									internalTrafficDesc.IPv4Address = parts[0]
									internalTrafficDesc.IPv4Mask = parts[1]
								}
							}
						case "OSAppId":
							// OS Id + OS App Id type (3GPP TS 24.526): 16-byte OS Id (UUID)
							// followed by the OS App Id string.
							internalTrafficDesc.Type = 0x08
							internalTrafficDesc.OSId = parseUUID(trafficDesc.OSId)
							internalTrafficDesc.OSAppId = trafficDesc.OSAppId
						}

						internalRule.TrafficDescriptors = append(internalRule.TrafficDescriptors, internalTrafficDesc)
					}

					// Convert route selection descriptors
					for _, routeSelDesc := range urspRule.RouteSelectionDescriptors {
						internalRouteDesc := RouteSelectionDescriptor{
							Precedence: routeSelDesc.Precedence,
							Components: make([]RouteSelectionComponent, 0),
						}

						// Convert components
						for _, component := range routeSelDesc.Components {
							internalComponent := RouteSelectionComponent{}

							switch component.Type {
							case "S-NSSAI":
								internalComponent.Type = 0x02
								// Parse S-NSSAI format like "1-000001"
								if strings.Contains(component.Value, "-") {
									parts := strings.Split(component.Value, "-")
									if len(parts) == 2 {
										sst, _ := strconv.ParseUint(parts[0], 10, 8)
										sd, _ := strconv.ParseUint(parts[1], 16, 32)
										internalComponent.SNSSAI = &SNSSAI{SST: uint8(sst), SD: uint32(sd)}
									}
								}
							case "DNN":
								internalComponent.Type = 0x04
								internalComponent.DNN = component.Value
							case "PDUSessionType":
								internalComponent.Type = 0x08
								sessionType, _ := strconv.ParseUint(component.Value, 10, 8)
								internalComponent.PDUSessionType = uint8(sessionType)
							case "PreferredAccessType":
								internalComponent.Type = 0x10
								if component.Value == "Non-3GPP" {
									internalComponent.PreferredAccessType = 2
								} else {
									internalComponent.PreferredAccessType = 1
								}
							}

							internalRouteDesc.Components = append(internalRouteDesc.Components, internalComponent)
						}

						internalRule.RouteSelectionDescs = append(internalRule.RouteSelectionDescs, internalRouteDesc)
					}

					internalInstruction.PolicyPart.URSPRules = append(internalInstruction.PolicyPart.URSPRules, internalRule)
				}
			}
		}

		internalConfig.Instructions = append(internalConfig.Instructions, internalInstruction)
	}

	return internalConfig, nil
}

// BuildManageUEPolicyCommand builds the Manage UE Policy Command payload
func BuildManageUEPolicyCommand(policyConfig *UEPolicyConfig) ([]byte, error) {
	logger.UePolicyLog.Info("WNC: Building Manage UE Policy Command")

	if policyConfig == nil {
		return nil, fmt.Errorf("WNC: Policy configuration is nil")
	}

	var payload []byte

	// Procedure Transaction Identity
	payload = append(payload, policyConfig.ProcedureTransID)

	// Message Type: MANAGE UE POLICY COMMAND (0x01)
	payload = append(payload, 0x01)

	// Build UE policy section management list
	policyListPayload, err := buildUEPolicyList(policyConfig)
	if err != nil {
		return nil, fmt.Errorf("WNC: Failed to build UE policy list: %v", err)
	}

	// UE policy section management list length (2 bytes, big-endian)
	listLen := uint16(len(policyListPayload))
	payload = append(payload, uint8(listLen>>8))
	payload = append(payload, uint8(listLen))
	payload = append(payload, policyListPayload...)

	logger.UePolicyLog.Infof("WNC: Built Manage UE Policy Command with %d bytes", len(payload))
	return payload, nil
}

func buildUEPolicyList(config *UEPolicyConfig) ([]byte, error) {
	var payload []byte

	// Build PLMN sublist
	plmnPayload, err := buildPLMNSublist(config)
	if err != nil {
		return nil, err
	}

	// PLMN sublist length (2 bytes, big-endian)
	sublistLen := uint16(len(plmnPayload))
	payload = append(payload, uint8(sublistLen>>8))
	payload = append(payload, uint8(sublistLen))
	payload = append(payload, plmnPayload...)

	return payload, nil
}

func buildPLMNSublist(config *UEPolicyConfig) ([]byte, error) {
	var payload []byte

	// Encode PLMN (MCC + MNC)
	plmnBytes, err := encodePLMN(config.PLMN.MCC, config.PLMN.MNC)
	if err != nil {
		return nil, err
	}
	payload = append(payload, plmnBytes...)

	// Build instructions
	for _, instruction := range config.Instructions {
		instructionPayload, err := buildInstruction(&instruction)
		if err != nil {
			return nil, err
		}

		// Instruction length (2 bytes, big-endian)
		instrLen := uint16(len(instructionPayload))
		payload = append(payload, uint8(instrLen>>8))
		payload = append(payload, uint8(instrLen))
		payload = append(payload, instructionPayload...)
	}

	return payload, nil
}

func buildInstruction(instruction *InstructionConfig) ([]byte, error) {
	var payload []byte

	// UPSC (2 bytes, big-endian)
	upsc := instruction.UPSC
	payload = append(payload, uint8(upsc>>8))
	payload = append(payload, uint8(upsc))

	// WNC: Type 0 means the YAML instruction had no policyParts. Empty UE policy section
	// contents tell the UE to delete the section identified by PLMN ID + UPSC.
	if instruction.PolicyPart.Type == 0 {
		return payload, nil
	}

	// Build UE policy part
	policyPartPayload, err := buildUEPolicyPart(&instruction.PolicyPart)
	if err != nil {
		return nil, err
	}

	// UE policy part length (2 bytes, big-endian)
	partLen := uint16(len(policyPartPayload))
	payload = append(payload, uint8(partLen>>8))
	payload = append(payload, uint8(partLen))
	payload = append(payload, policyPartPayload...)

	return payload, nil
}

func buildUEPolicyPart(policyPart *UEPolicyPartConfig) ([]byte, error) {
	var payload []byte

	// UE policy part type
	payload = append(payload, policyPart.Type)

	if policyPart.Type == 1 { // URSP
		urspPayload, err := buildURSPRules(policyPart.URSPRules)
		if err != nil {
			return nil, err
		}
		payload = append(payload, urspPayload...)
	}

	return payload, nil
}

func buildURSPRules(rules []URSPRule) ([]byte, error) {
	var payload []byte

	for _, rule := range rules {
		rulePayload, err := buildURSPRule(&rule)
		if err != nil {
			return nil, err
		}

		// URSP rule length (2 bytes, big-endian per Wireshark dissector)
		ruleLen := uint16(len(rulePayload))
		payload = append(payload, uint8(ruleLen>>8))
		payload = append(payload, uint8(ruleLen))
		payload = append(payload, rulePayload...)
	}

	return payload, nil
}

func buildURSPRule(rule *URSPRule) ([]byte, error) {
	var payload []byte

	// Precedence (1 byte)
	payload = append(payload, rule.Precedence)

	// Build traffic descriptors
	trafficPayload, err := buildTrafficDescriptors(rule.TrafficDescriptors)
	if err != nil {
		return nil, err
	}

	// Traffic descriptor length (2 bytes, big-endian per Wireshark dissector)
	trafficLen := uint16(len(trafficPayload))
	payload = append(payload, uint8(trafficLen>>8))
	payload = append(payload, uint8(trafficLen))
	payload = append(payload, trafficPayload...)

	// Build route selection descriptors
	routePayload, err := buildRouteSelectionDescriptors(rule.RouteSelectionDescs)
	if err != nil {
		return nil, err
	}

	// Route selection descriptor length (2 bytes, big-endian per Wireshark dissector)
	routeLen := uint16(len(routePayload))
	payload = append(payload, uint8(routeLen>>8))
	payload = append(payload, uint8(routeLen))
	payload = append(payload, routePayload...)

	return payload, nil
}

func buildTrafficDescriptors(descriptors []TrafficDescriptor) ([]byte, error) {
	var payload []byte

	for _, desc := range descriptors {
		switch desc.Type {
		case 1: // Match-all - per Wireshark dissector, this should be the only descriptor
			payload = append(payload, 1)
			// Per Wireshark case 0x01: Match-all type, return immediately
			return payload, nil

		case 8: // OS Id + OS App Id type (3GPP TS 24.526)
			payload = append(payload, 8)

			// OS Id: 16-byte UUID
			payload = append(payload, desc.OSId...)

			// OS App Id: 1-byte length followed by the ASCII string
			appIdBytes := []byte(desc.OSAppId)
			payload = append(payload, uint8(len(appIdBytes)))
			payload = append(payload, appIdBytes...)

		case 16: // IPv4 remote address
			payload = append(payload, 16)

			// Parse IPv4 address and mask
			ipBytes := parseIPv4(desc.IPv4Address)
			maskBytes := parseIPv4(desc.IPv4Mask)

			// Length: 8 bytes (4 for IP + 4 for mask)
			payload = append(payload, 8)
			payload = append(payload, ipBytes...)
			payload = append(payload, maskBytes...)

		case 136: // DNN
			payload = append(payload, 136)

			// DNN encoded in APN format (length-prefixed labels)
			dnnBytes := encodeDNNAsAPN(desc.DNN)
			payload = append(payload, uint8(len(dnnBytes)))
			payload = append(payload, dnnBytes...)

		default:
			return nil, fmt.Errorf("WNC: Unsupported traffic descriptor type: %d", desc.Type)
		}
	}

	return payload, nil
}

func buildRouteSelectionDescriptors(descriptors []RouteSelectionDescriptor) ([]byte, error) {
	var payload []byte

	for _, desc := range descriptors {
		descPayload, err := buildRouteSelectionDescriptor(&desc)
		if err != nil {
			return nil, err
		}

		// Route selection descriptor length (2 bytes, big-endian per Wireshark dissector)
		descLen := uint16(len(descPayload))
		payload = append(payload, uint8(descLen>>8))
		payload = append(payload, uint8(descLen))
		payload = append(payload, descPayload...)
	}

	return payload, nil
}

func buildRouteSelectionDescriptor(desc *RouteSelectionDescriptor) ([]byte, error) {
	var payload []byte

	// Precedence (1 byte)
	payload = append(payload, desc.Precedence)

	// Build components
	componentsPayload, err := buildRouteSelectionComponents(desc.Components)
	if err != nil {
		return nil, err
	}

	// Components length (2 bytes, big-endian per Wireshark dissector)
	compLen := uint16(len(componentsPayload))
	payload = append(payload, uint8(compLen>>8))
	payload = append(payload, uint8(compLen))
	payload = append(payload, componentsPayload...)

	return payload, nil
}

func buildRouteSelectionComponents(components []RouteSelectionComponent) ([]byte, error) {
	var payload []byte

	for _, comp := range components {
		switch comp.Type {
		case 2: // S-NSSAI
			payload = append(payload, 2)
			payload = append(payload, 4) // Length: SST(1) + SD(3)
			payload = append(payload, comp.SNSSAI.SST)

			// SD as 3 bytes (big endian)
			payload = append(payload, uint8(comp.SNSSAI.SD>>16))
			payload = append(payload, uint8(comp.SNSSAI.SD>>8))
			payload = append(payload, uint8(comp.SNSSAI.SD))

		case 4: // DNN
			payload = append(payload, 4)
			dnnBytes := encodeDNNAsAPN(comp.DNN)
			payload = append(payload, uint8(len(dnnBytes)))
			payload = append(payload, dnnBytes...)

		case 8: // PDU session type
			payload = append(payload, 8)
			payload = append(payload, comp.PDUSessionType)

		case 16: // Preferred access type
			payload = append(payload, 16)
			payload = append(payload, comp.PreferredAccessType)

		default:
			return nil, fmt.Errorf("WNC: Unsupported route selection component type: %d", comp.Type)
		}
	}

	return payload, nil
}

func encodePLMN(mcc, mnc string) ([]byte, error) {
	if len(mcc) != 3 || len(mnc) < 2 || len(mnc) > 3 {
		return nil, fmt.Errorf("WNC: Invalid PLMN format - MCC: %s, MNC: %s", mcc, mnc)
	}

	// Parse MCC digits
	mcc1 := mcc[0] - '0'
	mcc2 := mcc[1] - '0'
	mcc3 := mcc[2] - '0'

	// Parse MNC digits
	mnc1 := mnc[0] - '0'
	mnc2 := mnc[1] - '0'
	var mnc3 uint8 = 0xF // Default for 2-digit MNC

	if len(mnc) == 3 {
		mnc3 = mnc[2] - '0'
	}

	// Encode as per 3GPP TS 24.008
	plmn := make([]byte, 3)
	plmn[0] = (mcc2 << 4) | mcc1
	plmn[1] = (mnc3 << 4) | mcc3
	plmn[2] = (mnc2 << 4) | mnc1

	logger.UePolicyLog.Infof("WNC: Encoded PLMN %s-%s to %02x%02x%02x", mcc, mnc, plmn[0], plmn[1], plmn[2])
	return plmn, nil
}

func parseIPv4(ipStr string) []byte {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		logger.UePolicyLog.Warnf("WNC: Failed to parse IPv4 address: %s", ipStr)
		return []byte{0, 0, 0, 0}
	}
	return ip.To4()
}

// parseUUID converts a UUID string (e.g. "97a498e3-fc92-5c94-8986-0333d06e4e47")
// into its 16-byte representation for the OS Id of a UE policy traffic descriptor.
// On any parse error it logs a warning and returns 16 zero bytes.
func parseUUID(uuid string) []byte {
	hexStr := strings.ReplaceAll(uuid, "-", "")
	b, err := hex.DecodeString(hexStr)
	if err != nil || len(b) != 16 {
		logger.UePolicyLog.Warnf("WNC: Failed to parse OS Id UUID %q (decoded %d bytes): %v", uuid, len(b), err)
		return make([]byte, 16)
	}
	return b
}

// encodeDNNAsAPN encodes DNN string in APN format as defined in 3GPP TS 23.003
// Each label is prefixed with its length (similar to DNS names)
func encodeDNNAsAPN(dnn string) []byte {
	if dnn == "" {
		return []byte{}
	}

	// Split DNN by dots (e.g., "internet.example.com" -> ["internet", "example", "com"])
	labels := strings.Split(dnn, ".")
	var encoded []byte

	for _, label := range labels {
		if len(label) > 0 && len(label) <= 255 {
			// Prefix each label with its length
			encoded = append(encoded, uint8(len(label)))
			encoded = append(encoded, []byte(label)...)
		}
	}

	// No null terminator needed for DNN encoding in UE policy
	return encoded
}
