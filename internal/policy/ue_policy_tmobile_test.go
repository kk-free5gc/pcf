package policy

import (
	"encoding/hex"
	"testing"
)

// Reference "Manage UE Policy Command" payload (PTI onward, 245 bytes) that
// BuildManageUEPolicyCommand must produce from config/pcfcfg_ue_policy_tmo.yaml, so that
// the PCF delivers the intended URSP instead of an empty command.
//
// The payload started as a 170-byte capture from a real T-Mobile US network via QXDM.
// The YAML file has since been changed in three ways, and these bytes follow the YAML file:
//   - UPSC 3001 (0b b9) instead of the capture's 2001 (07 d1).
//   - Rule 3 added: PRIORITIZE_GAMING -> S-NSSAI 1-000bb9.
//   - Each route selection descriptor ends 08 03 (PDUSessionType IPv4v6) instead of the
//     capture's 08 02 (IPv6-only). See the commit 66c5eb8 note in ue_policy_00101_test.go.
//
// Load the _tmo variant by name, NOT the pcfcfg_ue_policy_wnc.yaml symlink: that symlink
// is the swappable "active variant" pointer the NFs read at runtime, and the PLMN it
// selects is encoded into the payload. See ue_policy_00101_test.go for the other variant.
const tmobileReferenceHex = "800100f100ef13006200ea0bb900e601004a0100240897a498e3fc925c9489860333" +
	"d06e4e47125052494f524954495a455f4c4154454e43590021001f01001c0204010007d00412046661737408742d" +
	"6d6f62696c6503636f6d0803004c0200260897a498e3fc925c9489860333d06e4e47145052494f524954495a455f" +
	"42414e4457494454480021001f01001c020401000bb80412046661737408742d6d6f62696c6503636f6d08030049" +
	"0300230897a498e3fc925c9489860333d06e4e47115052494f524954495a455f47414d494e470021001f01001c02" +
	"0401000bb90412046661737408742d6d6f62696c6503636f6d0803"

func TestBuildManageUEPolicyCommand_TMobile(t *testing.T) {
	want, err := hex.DecodeString(tmobileReferenceHex)
	if err != nil {
		t.Fatalf("bad reference hex: %v", err)
	}

	cfg, err := LoadUEPolicyConfigFromYAML("../../../../config/pcfcfg_ue_policy_tmo.yaml")
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
