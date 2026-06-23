package budget

import "testing"

// Spec §8.4 anchors: 80% is the warn threshold, 100% selects the
// bucket-specific terminal rung, and unknown buckets fall through to Block.
func TestDegradePath_SpecTable(t *testing.T) {
	cases := []struct {
		name   string
		bucket Bucket
		ratio  float64
		want   DegradeAction
	}{
		{"below_warn", BucketCloudEmbedBytes, 0.5, ActionNone},
		{"just_below_warn", BucketCloudEmbedBytes, 0.799, ActionNone},
		{"at_warn", BucketCloudEmbedBytes, 0.80, ActionWarn},
		{"mid_warn", BucketCloudVisionBytes, 0.85, ActionWarn},
		{"just_below_cap", BucketCloudTTSChars, 0.999, ActionWarn},
		{"at_cap_embed", BucketCloudEmbedBytes, 1.00, ActionSwitchLocal},
		{"at_cap_stt", BucketCloudSTTSeconds, 1.00, ActionSwitchLocal},
		{"at_cap_tts", BucketCloudTTSChars, 1.00, ActionSwitchLocal},
		{"at_cap_vision", BucketCloudVisionBytes, 1.00, ActionSwitchLocal},
		{"at_cap_plugin", BucketPluginNetworkBytes, 1.00, ActionPause},
		{"at_cap_peer_a2a", BucketPeerA2ATokens, 1.00, ActionBlock},
		{"at_cap_llm", BucketCloudLLMTokens, 1.00, ActionBlock},
		{"over_cap_embed", BucketCloudEmbedBytes, 1.5, ActionSwitchLocal},
		{"over_cap_plugin_escalates_to_disable", BucketPluginNetworkBytes, 1.5, ActionDisable},
		{"over_cap_peer_a2a", BucketPeerA2ATokens, 1.5, ActionBlock},
		{"unknown_bucket", Bucket("cloud.unknown"), 1.0, ActionBlock},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DegradePath(c.bucket, c.ratio); got != c.want {
				t.Errorf("DegradePath(%q, %v) = %d, want %d", c.bucket, c.ratio, got, c.want)
			}
		})
	}
}
