import { For } from 'solid-js'

// The app's icon, drawn from the layers in icon/AppIcon.icon: a record on
// a red card, its red label and white tonearm. The card follows the theme.
export function AppIcon(props: { size?: number; class?: string }) {
  const grooves = Array.from({ length: 24 }, (_, i) => 126 + i * 7)
  return (
    <svg
      class={props.class}
      width={props.size ?? 32}
      height={props.size ?? 32}
      viewBox="0 0 1024 1024"
      aria-hidden="true"
    >
      <defs>
        <linearGradient id="icon-card" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" style={{ 'stop-color': 'var(--icon-top)' }} />
          <stop offset="1" style={{ 'stop-color': 'var(--icon-foot)' }} />
        </linearGradient>
        <radialGradient id="icon-disc" cx="470" cy="548" r="300" gradientUnits="userSpaceOnUse">
          <stop offset="0.35" stop-color="#2A2A30" />
          <stop offset="0.95" stop-color="#17171B" />
          <stop offset="1" stop-color="#0E0E11" />
        </radialGradient>
        <linearGradient id="icon-red" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stop-color="#FF5B5B" />
          <stop offset="1" stop-color="#E3122C" />
        </linearGradient>
        <linearGradient id="icon-arm" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stop-color="#FFFFFF" />
          <stop offset="1" stop-color="#E6E3E5" />
        </linearGradient>
        <linearGradient id="icon-sheen" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stop-color="#fff" stop-opacity="0" />
          <stop offset="0.5" stop-color="#fff" stop-opacity="0.2" />
          <stop offset="1" stop-color="#fff" stop-opacity="0" />
        </linearGradient>
      </defs>
      <rect width="1024" height="1024" rx="230" fill="url(#icon-card)" />
      <circle cx="470" cy="548" r="300" fill="url(#icon-disc)" />
      <circle cx="470" cy="548" r="292" fill="none" stroke="#fff" stroke-opacity="0.14" stroke-width="3" />
      <For each={grooves}>
        {(r, i) => (
          <circle
            cx="470"
            cy="548"
            r={r}
            fill="none"
            stroke="#fff"
            stroke-opacity={i() % 2 ? 0.07 : 0.04}
            stroke-width="1.4"
          />
        )}
      </For>
      <path d={wedge(2, 48)} fill="url(#icon-sheen)" />
      <path d={wedge(182, 228)} fill="url(#icon-sheen)" opacity="0.7" />
      <circle cx="470" cy="548" r="107.4" fill="url(#icon-red)" />
      <circle cx="470" cy="548" r="74.1" fill="none" stroke="#fff" stroke-opacity="0.28" stroke-width="3" />
      <circle cx="470" cy="548" r="15.6" fill="#FFF6F2" />
      <g transform="rotate(110.59 772 252)">
        <rect x="688" y="228" width="60" height="48" rx="14" fill="url(#icon-arm)" />
        <rect x="772" y="240" width="407.97" height="24" rx="12" fill="url(#icon-arm)" />
        <rect
          x="1159.97"
          y="226"
          width="84"
          height="52"
          rx="13"
          fill="url(#icon-arm)"
          transform="rotate(28 1179.97 252)"
        />
      </g>
      <circle cx="772" cy="252" r="54" fill="url(#icon-arm)" />
      <circle cx="772" cy="252" r="20" fill="#D9D5D8" />
    </svg>
  )
}

// wedge is the path of the record's sheen between two angles, in degrees
// clockwise from 12 o'clock, from the label's edge to the rim.
function wedge(from: number, to: number) {
  const at = (deg: number, r: number) => {
    const a = ((deg - 90) * Math.PI) / 180
    return `${(470 + r * Math.cos(a)).toFixed(2)} ${(548 + r * Math.sin(a)).toFixed(2)}`
  }
  return `M${at(from, 117)}L${at(from, 290)}A290 290 0 0 1 ${at(to, 290)}L${at(to, 117)}A117 117 0 0 0 ${at(from, 117)}Z`
}
