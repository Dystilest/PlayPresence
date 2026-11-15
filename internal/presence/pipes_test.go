package presence

import (
	"slices"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestPipeCandidatesWindows(t *testing.T) {
	got := PipeCandidates("windows", env(nil))

	if len(got) != 10 {
		t.Fatalf("got %d candidates, want 10", len(got))
	}
	if got[0] != `\\?\pipe\discord-ipc-0` || got[9] != `\\?\pipe\discord-ipc-9` {
		t.Errorf("unexpected pipes: %v", got)
	}
}

func TestPipeCandidatesLinuxIncludesSandboxedClients(t *testing.T) {
	got := PipeCandidates("linux", env(map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000"}))

	for _, want := range []string{
		"/run/user/1000/discord-ipc-0",
		"/run/user/1000/app/com.discordapp.Discord/discord-ipc-0",
		"/run/user/1000/.flatpak/com.discordapp.Discord/xdg-run/discord-ipc-0",
		"/run/user/1000/.flatpak/dev.vencord.Vesktop/xdg-run/discord-ipc-0",
		"/run/user/1000/snap.discord/discord-ipc-0",
		"/run/user/1000/discord-ipc-9",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("missing candidate %q", want)
		}
	}
	if got[0] != "/run/user/1000/discord-ipc-0" {
		t.Errorf("plain socket must come first, got %q", got[0])
	}
}

func TestPipeCandidatesFallbackOrder(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"xdg wins", map[string]string{"XDG_RUNTIME_DIR": "/x", "TMPDIR": "/t"}, "/x/discord-ipc-0"},
		{"tmpdir", map[string]string{"TMPDIR": "/var/folders/ab"}, "/var/folders/ab/discord-ipc-0"},
		{"tmp", map[string]string{"TMP": "/tmpx"}, "/tmpx/discord-ipc-0"},
		{"temp", map[string]string{"TEMP": "/tempx"}, "/tempx/discord-ipc-0"},
		{"default", nil, "/tmp/discord-ipc-0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PipeCandidates("darwin", env(tt.env))

			if got[0] != tt.want {
				t.Errorf("first candidate = %q, want %q", got[0], tt.want)
			}
		})
	}
}
