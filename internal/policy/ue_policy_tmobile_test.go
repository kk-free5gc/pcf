package policy

import (
	"encoding/hex"
	"testing"
)

// Reference "Manage UE Policy Command" payload (PTI onward, 170 bytes) captured from a
// real T-Mobile US network via QXDM. This is the exact byte string
// BuildManageUEPolicyCommand must produce from config/pcfcfg_ue_policy_wnc.yaml so that
// the PCF delivers the intended URSP instead of an empty command.
const tmobileReferenceHex = "800100a600a4130062009f07d1009b01004a0100240897a498e3fc925c9489860333" +
	"d06e4e47125052494f524954495a455f4c4154454e43590021001f01001c0204010007d00412046661737408742d" +
	"6d6f62696c6503636f6d0802004c0200260897a498e3fc925c9489860333d06e4e47145052494f524954495a455f4" +
	"2414e4457494454480021001f01001c020401000bb80412046661737408742d6d6f62696c6503636f6d0802"

func TestBuildManageUEPolicyCommand_TMobile(t *testing.T) {
	want, err := hex.DecodeString(tmobileReferenceHex)
	if err != nil {
		t.Fatalf("bad reference hex: %v", err)
	}

	cfg, err := LoadUEPolicyConfigFromYAML("../../../../config/pcfcfg_ue_policy_wnc.yaml")
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
