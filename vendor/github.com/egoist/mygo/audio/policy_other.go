//go:build !((darwin && !ios) || (((linux && !android) || windows) && (amd64 || arm64)))

package audio

// Browsers, mobile systems and unsupported backends retain fixed-rate output.
const desktopAudioFeatures = false
