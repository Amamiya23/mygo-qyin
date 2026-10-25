import { createSignal } from 'solid-js'

// The song the window in the hero plays.
export const track = {
  title: '春日影',
  artist: 'MyGO!!!!!',
  duration: 258,
}

const [playing, setPlaying] = createSignal(false)
const [position, setPosition] = createSignal(62)
const [liked, setLiked] = createSignal(false)
export { playing, position, liked, setLiked }

let frame = 0
let last = 0

function tick(now: number) {
  const dt = Math.min((now - last) / 1000, 0.1)
  last = now
  let p = position() + dt
  if (p >= track.duration) p = 0
  setPosition(p)
  frame = requestAnimationFrame(tick)
}

export function play() {
  if (playing()) return
  setPlaying(true)
  last = performance.now()
  frame = requestAnimationFrame(tick)
}

export function pause() {
  setPlaying(false)
  cancelAnimationFrame(frame)
}

export function toggle() {
  playing() ? pause() : play()
}

export function seek(fraction: number) {
  setPosition(Math.max(0, Math.min(1, fraction)) * track.duration)
}

export function clock(seconds: number) {
  const s = Math.floor(seconds)
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}
