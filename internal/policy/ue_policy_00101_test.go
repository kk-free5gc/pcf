package policy

import (
	"encoding/hex"
	"testing"
)

// Reference "Manage UE Policy Command" payload for the PLMN 00101 test network
// (config/pcfcfg_ue_policy_00101.yaml, used by the IMSI 001010000010548 SIM).
//
// NOTE: this differs from tmobileReferenceHex only at offset 6..8, the encoded PLMN:
//   310/260 -> 13 00 62, 001/01 -> 00 f1 10
//   (2-digit MNC, so the mnc3 nibble is the 0xF filler; see encodePLMN:
//   plmn[0]=(mcc2<<4)|mcc1, plmn[1]=(mnc3<<4)|mcc3, plmn[2]=(mnc2<<4)|mnc1)
//
// Both payloads end each route selection descriptor with 08 03 (PDUSessionType IPv4v6)
// instead of the QXDM capture's 08 02 (IPv6-only). That is the commit 66c5eb8 workaround:
// an IPv6-only session has no usable address until SLAAC completes, and without a timely
// RA the UE released the PDU session ~12s in with 5GSM cause #36.
const plmn00101ReferenceHex = "800100f100ef00f11000ea0bb900e601004a0100240897a498e3fc925c9489860333" +
	"d06e4e47125052494f524954495a455f4c4154454e43590021001f01001c0204010007d00412046661737408742d" +
	"6d6f62696c6503636f6d0803004c0200260897a498e3fc925c9489860333d06e4e47145052494f524954495a455f" +
	"42414e4457494454480021001f01001c020401000bb80412046661737408742d6d6f62696c6503636f6d08030049" +
	"0300230897a498e3fc925c9489860333d06e4e47115052494f524954495a455f47414d494e470021001f01001c02" +
	"0401000bb90412046661737408742d6d6f62696c6503636f6d0803"

func TestBuildManageUEPolicyCommand_00101(t *testing.T) {
	want, err := hex.DecodeString(plmn00101ReferenceHex)
	if err != nil {
		t.Fatalf("bad reference hex: %v", err)
	}

	cfg, err := LoadUEPolicyConfigFromYAML("../../../../config/pcfcfg_ue_policy_00101.yaml")
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

// The 00101 variant must stay a pure PLMN swap of the T-Mobile one: same URSP rules,
// same slices, same DNN, same PDU session types. This builds both payloads from their
// YAML files rather than comparing the two golden constants, so it keeps working when
// the rules are intentionally changed in both files at once (and still fails loudly if
// only one of them is edited).
func TestPLMN00101IsPurePLMNSwapOfTMobile(t *testing.T) {
	build := func(path string) []byte {
		t.Helper()
		cfg, err := LoadUEPolicyConfigFromYAML(path)
		if err != nil {
			t.Fatalf("LoadUEPolicyConfigFromYAML(%s): %v", path, err)
		}
		payload, err := BuildManageUEPolicyCommand(cfg)
		if err != nil {
			t.Fatalf("BuildManageUEPolicyCommand(%s): %v", path, err)
		}
		return payload
	}

	tmo := build("../../../../config/pcfcfg_ue_policy_tmo.yaml")
	other := build("../../../../config/pcfcfg_ue_policy_00101.yaml")

	if len(tmo) != len(other) {
		t.Fatalf("length mismatch: tmobile=%d 00101=%d", len(tmo), len(other))
	}

	// Offsets 6..8 are the encoded PLMN emitted by buildPLMNSublist.
	const plmnOff = 6
	for i := range tmo {
		if i >= plmnOff && i < plmnOff+3 {
			continue
		}
		if tmo[i] != other[i] {
			t.Fatalf("byte %d differs outside the PLMN field: tmobile=%02x 00101=%02x", i, tmo[i], other[i])
		}
	}

	if got := hex.EncodeToString(tmo[plmnOff : plmnOff+3]); got != "130062" {
		t.Fatalf("encoded PLMN for 310/260 = %s, want 130062", got)
	}
	if got := hex.EncodeToString(other[plmnOff : plmnOff+3]); got != "00f110" {
		t.Fatalf("encoded PLMN for 001/01 = %s, want 00f110", got)
	}
}
