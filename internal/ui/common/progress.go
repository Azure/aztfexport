package common

import "math/rand"

const ProgressShowLastResults = 5

func RandomHappyEmoji() string {
	emojis := []rune("🍦🍡🤠👾😭🦊🐯🦆🥨🎏🍔🍒🍥🎮📦🦁🐶🐸🍕🥐🧲🚒🥇🏆🌽")
	return string(emojis[rand.Intn(len(emojis))])
}
