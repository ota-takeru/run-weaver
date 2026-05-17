package cli

import "testing"

func TestDetectIssueDependencies(t *testing.T) {
	tests := []struct {
		name      string
		issue     githubIssue
		want      []int
		ambiguous bool
	}{
		{
			name:  "depends syntax",
			issue: githubIssue{Body: "depends: #10\nblocked by #11"},
			want:  []int{10, 11},
		},
		{
			name:  "japanese syntax",
			issue: githubIssue{Body: "#12 の後に実装"},
			want:  []int{12},
		},
		{
			name:  "text order",
			issue: githubIssue{Body: "blocked by #11 after #10"},
			want:  []int{11, 10},
		},
		{
			name:      "ambiguous dependency",
			issue:     githubIssue{Body: "depends on the auth cleanup"},
			ambiguous: true,
		},
		{
			name: "ignore run-weaver comments",
			issue: githubIssue{Comments: []githubComment{{
				Body: "run-weaver completed this issue.\n\n- draft PR: https://github.com/example/repo/pull/1\n- branch: codex/issue-1",
			}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ambiguous := detectIssueDependencies(tt.issue)
			if ambiguous != tt.ambiguous {
				t.Fatalf("ambiguous = %v, want %v", ambiguous, tt.ambiguous)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("dependencies = %#v, want %#v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("dependencies = %#v, want %#v", got, tt.want)
				}
			}
		})
	}
}
