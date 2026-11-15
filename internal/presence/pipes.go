package presence

import (
	"fmt"
	"path"
)

const pipeSlots = 10

// PipeCandidates lists the places a Discord client may expose its IPC
// endpoint, most likely first. getenv is os.Getenv in production.
func PipeCandidates(goos string, getenv func(string) string) []string {
	out := make([]string, 0, pipeSlots*5)

	if goos == "windows" {
		for i := 0; i < pipeSlots; i++ {
			out = append(out, fmt.Sprintf(`\\?\pipe\discord-ipc-%d`, i))
		}
		return out
	}

	base := "/tmp"
	for _, key := range []string{"XDG_RUNTIME_DIR", "TMPDIR", "TMP", "TEMP"} {
		if v := getenv(key); v != "" {
			base = v
			break
		}
	}
	for i := 0; i < pipeSlots; i++ {
		name := fmt.Sprintf("discord-ipc-%d", i)
		out = append(out,
			path.Join(base, name),
			path.Join(base, "app", "com.discordapp.Discord", name),
			path.Join(base, ".flatpak", "com.discordapp.Discord", "xdg-run", name),
			path.Join(base, ".flatpak", "dev.vencord.Vesktop", "xdg-run", name),
			path.Join(base, "snap.discord", name),
		)
	}
	return out
}
