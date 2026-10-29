// The same editable artwork as icon/AppIcon.icon and resources/icon.png.
export function AppIcon(props: { size?: number; class?: string }) {
  return (
    <svg
      class={props.class}
      width={props.size ?? 32}
      height={props.size ?? 32}
      viewBox="0 0 1024 1024"
      aria-hidden="true"
    >
      <defs>
        <linearGradient id="icon-card" x1="88" y1="64" x2="940" y2="960" gradientUnits="userSpaceOnUse">
          <stop stop-color="#D6F1DF" />
          <stop offset="1" stop-color="#30A46C" />
        </linearGradient>
        <radialGradient id="icon-vinyl" cx="322" cy="414" r="511" gradientUnits="userSpaceOnUse">
          <stop stop-color="#373B39" />
          <stop offset="0.92" stop-color="#171918" />
          <stop offset="1" stop-color="#101211" />
        </radialGradient>
        <linearGradient id="icon-label" x1="292" y1="450" x2="532" y2="672" gradientUnits="userSpaceOnUse">
          <stop stop-color="#FFEE9C" />
          <stop offset="1" stop-color="#FFC53D" />
        </linearGradient>
        <linearGradient id="icon-arm" x1="625" y1="230" x2="842" y2="620" gradientUnits="userSpaceOnUse">
          <stop stop-color="#FFFFFF" />
          <stop offset="1" stop-color="#E6E9E8" />
        </linearGradient>
        <clipPath id="icon-shape">
          <rect width="1024" height="1024" rx="216" />
        </clipPath>
      </defs>
      <g clip-path="url(#icon-shape)">
        <rect width="1024" height="1024" fill="url(#icon-card)" />
        <circle cx="426" cy="572" r="380" fill="#193B2D" opacity="0.16" />
        <circle cx="410" cy="555" r="363" fill="url(#icon-vinyl)" />
        <circle cx="410" cy="555" r="319" fill="none" stroke="#63706B" stroke-width="8" />
        <circle cx="410" cy="555" r="261" fill="none" stroke="#444947" stroke-width="8" />
        <circle cx="410" cy="555" r="197" fill="none" stroke="#5B625F" stroke-width="8" />
        <path d="M131 510A299 299 0 0 1 266 329M551 780A299 299 0 0 0 669 620" fill="none" stroke="#ADB5B2" stroke-opacity="0.48" stroke-width="22" stroke-linecap="round" />
        <circle cx="410" cy="555" r="136" fill="url(#icon-label)" />
        <circle cx="410" cy="555" r="30" fill="#1A211E" />
        <path d="M832 263V438Q832 483 797 518L677 620" fill="none" stroke="#193B2D" stroke-opacity="0.2" stroke-width="72" stroke-linecap="round" stroke-linejoin="round" />
        <path d="M820 245V420Q820 465 785 500L665 602" fill="none" stroke="url(#icon-arm)" stroke-width="72" stroke-linecap="round" stroke-linejoin="round" />
        <circle cx="820" cy="235" r="91" fill="url(#icon-arm)" />
        <circle cx="820" cy="235" r="36" fill="#FFC53D" />
        <path d="M660 524L730 594L655 676Q638 694 620 676L588 644Q570 626 588 608Z" fill="#FFC53D" />
      </g>
    </svg>
  )
}
