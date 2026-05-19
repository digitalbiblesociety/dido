package pipeline

import "testing"

// SAB always writes os_task_file_head_tail_format in its config
// string; dido must recognise it (not warn-as-unknown).
func TestParseTaskConfig_HeadTailFormat(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"hidden (SAB default)", "task_language=eng|os_task_file_head_tail_format=hidden", "hidden"},
		{"add (aeneas default)", "task_language=eng|os_task_file_head_tail_format=add", "add"},
		{"stretch", "task_language=eng|os_task_file_head_tail_format=stretch", "stretch"},
		{"short alias", "task_language=eng|o_h_t_format=hidden", "hidden"},
		{"absent → empty string", "task_language=eng", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, unknown := ParseTaskConfigStrict(tc.in)
			if cfg.HeadTailFormat != tc.want {
				t.Errorf("HeadTailFormat = %q, want %q", cfg.HeadTailFormat, tc.want)
			}
			for _, k := range unknown {
				if k == "os_task_file_head_tail_format" || k == "o_h_t_format" {
					t.Errorf("key %q reported as unknown despite being recognised", k)
				}
			}
		})
	}
}
