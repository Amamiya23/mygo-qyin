package app

import "github.com/egoist/mygo/ui"

// Radix Colors, MIT © WorkOS. The neutral and accent scales share the
// documented roles: 1–2 backgrounds, 3–5 controls, 6–8 borders, 9–10 solid
// accents, and 11–12 text. Keep the official sRGB values here so native UI
// uses the same palette in both appearances.
// https://www.radix-ui.com/colors/docs/palette-composition/understanding-the-scale
var (
	radixGrayLight  = [...]string{"#fcfcfc", "#f9f9f9", "#f0f0f0", "#e8e8e8", "#e0e0e0", "#d9d9d9", "#cecece", "#bbbbbb", "#8d8d8d", "#838383", "#646464", "#202020"}
	radixGrayDark   = [...]string{"#111111", "#191919", "#222222", "#2a2a2a", "#313131", "#3a3a3a", "#484848", "#606060", "#6e6e6e", "#7b7b7b", "#b4b4b4", "#eeeeee"}
	radixSageLight  = [...]string{"#fbfdfc", "#f7f9f8", "#eef1f0", "#e6e9e8", "#dfe2e0", "#d7dad9", "#cbcfcd", "#b8bcba", "#868e8b", "#7c8481", "#5f6563", "#1a211e"}
	radixSageDark   = [...]string{"#101211", "#171918", "#202221", "#272a29", "#2e3130", "#373b39", "#444947", "#5b625f", "#63706b", "#717d79", "#adb5b2", "#eceeed"}
	radixGreenLight = [...]string{"#fbfefc", "#f4fbf6", "#e6f6eb", "#d6f1df", "#c4e8d1", "#adddc0", "#8eceaa", "#5bb98b", "#30a46c", "#2b9a66", "#218358", "#193b2d"}
	radixGreenDark  = [...]string{"#0e1512", "#121b17", "#132d21", "#113b29", "#174933", "#20573e", "#28684a", "#2f7c57", "#30a46c", "#33b074", "#3dd68c", "#b1f1cb"}
	radixRedLight   = [...]string{"#fffcfc", "#fff7f7", "#feebec", "#ffdbdc", "#ffcdce", "#fdbdbe", "#f4a9aa", "#eb8e90", "#e5484d", "#dc3e42", "#ce2c31", "#641723"}
	radixRedDark    = [...]string{"#191111", "#201314", "#3b1219", "#500f1c", "#611623", "#72232d", "#8c333a", "#b54548", "#e5484d", "#ec5d5e", "#ff9592", "#ffd1d9"}
	radixAmberLight = [...]string{"#fefdfb", "#fefbe9", "#fff7c2", "#ffee9c", "#fbe577", "#f3d673", "#e9c162", "#e2a336", "#ffc53d", "#ffba18", "#ab6400", "#4f3422"}
	radixAmberDark  = [...]string{"#16120c", "#1d180f", "#302008", "#3f2700", "#4d3000", "#5c3d05", "#714f19", "#8f6424", "#ffc53d", "#ffd60a", "#ffca16", "#ffe7b3"}
	radixAlpha      = [...]float32{0.05, 0.1, 0.15, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 0.95}

	// Black and white are the opaque endpoints of Radix's overlay scales;
	// they also provide fixed contrast for media and QR codes.
	radixInk   = ui.Hex("#000000")
	radixPaper = ui.Hex("#ffffff")

	// Step 9 colors used by decorative celebration particles.
	radixPink9   = ui.Hex("#d6409f")
	radixOrange9 = ui.Hex("#f76b15")
	radixPurple9 = ui.Hex("#8e4ec6")
	radixCyan9   = ui.Hex("#00a2c7")
)

func radixGray(dark bool, step int) ui.Color {
	if dark {
		return ui.Hex(radixGrayDark[step-1])
	}
	return ui.Hex(radixGrayLight[step-1])
}

func radixSage(dark bool, step int) ui.Color {
	if dark {
		return ui.Hex(radixSageDark[step-1])
	}
	return ui.Hex(radixSageLight[step-1])
}

func radixGreen(dark bool, step int) ui.Color {
	if dark {
		return ui.Hex(radixGreenDark[step-1])
	}
	return ui.Hex(radixGreenLight[step-1])
}

func radixRed(dark bool, step int) ui.Color {
	if dark {
		return ui.Hex(radixRedDark[step-1])
	}
	return ui.Hex(radixRedLight[step-1])
}

func radixAmber(dark bool, step int) ui.Color {
	if dark {
		return ui.Hex(radixAmberDark[step-1])
	}
	return ui.Hex(radixAmberLight[step-1])
}

func radixBlack(step int) ui.Color { return radixInk.Alpha(radixAlpha[step-1]) }
func radixWhite(step int) ui.Color { return radixPaper.Alpha(radixAlpha[step-1]) }
