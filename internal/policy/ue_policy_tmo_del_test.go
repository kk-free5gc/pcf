package policy

import (
	"encoding/hex"
	"testing"
)

// Expected "Manage UE Policy Command" payload for config/pcfcfg_ue_policy_tmo_del.yaml.
// Each instruction in that file has an empty policyParts list, which asks the UE to delete
// the UE policy section named by PLMN 310260 + UPSC. The instruction must therefore hold
// the 2-byte UPSC only (instruction length 0x0002), with no UE policy part after it:
//
//	80        PTI
//	01        MANAGE UE POLICY COMMAND
//	000d      UE policy section management list length
//	000b      PLMN sublist length
//	130062    PLMN 310260
//	0002 07d1 instruction: UPSC 2001, empty contents
//	0002 0bb9 instruction: UPSC 3001, empty contents
const tmobileDeleteReferenceHex = "8001000d000b130062000207d100020bb9"

func TestBuildManageUEPolicyCommand_TMobileDelete(t *testing.T) {
	want, err := hex.DecodeString(tmobileDeleteReferenceHex)
	if err != nil {
		t.Fatalf("bad reference hex: %v", err)
	}

	cfg, err := LoadUEPolicyConfigFromYAML("../../../../config/pcfcfg_ue_policy_tmo_del.yaml")
	if err != nil {
		t.Fatalf("LoadUEPolicyConfigFromYAML: %v", err)
	}

	got, err := BuildManageUEPolicyCommand(cfg)
	if err != nil {
		t.Fatalf("BuildManageUEPolicyCommand: %v", err)
	}

	if hex.EncodeToString(got) != hex.EncodeToString(want) {
		t.Fatalf("payload mismatch\n want (%d): %s\n got  (%d): %s",
			len(want), hex.EncodeToString(want), len(got), hex.EncodeToString(got))
	}
}
