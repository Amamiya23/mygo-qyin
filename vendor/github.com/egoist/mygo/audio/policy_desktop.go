//go:build (darwin && !ios) || (((linux && !android) || windows) && (amd64 || arm64))

package audio

// desktopAudioFeatures is limited to the supported desktop backends.
const desktopAudioFeatures = true
