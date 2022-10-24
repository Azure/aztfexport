package common

import "math/rand"

const ProgressShowLastResults = 5

func RandomHappyEmoji() string {
	emojis := []rune("🍦🍡🤠👾😭🦊🐯🦆🥨🎏🍔🍒🍥🎮📦🦁🐶🐸🍕🥐🧲🚒🥇🏆🌽")
	// #nosec G404 -- This is fine for UI
	return string(emojis[rand.Intn(len(emojis))])
}
