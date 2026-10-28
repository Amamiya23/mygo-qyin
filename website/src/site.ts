export const repo = 'https://github.com/Amamiya23/mygo-qyin'

export type Asset = { name: string; size: number; browser_download_url: string }
export type Release = { tag_name: string; published_at: string; assets: Asset[] }

declare const __RELEASE__: Release
// The latest release, as it was when the site was built (see vite.config.ts).
export const release = __RELEASE__
export const mygo = 'https://mygo.egoist.dev'

export const site = {
  name: 'Lilt',
  title: 'Lilt — 原生的 QQ 音乐桌面客户端',
  description:
    '用 Go 与 MyGo 写成的第三方 QQ 音乐桌面客户端。原生界面，纯 Go 直连；边下边播、FLAC 无损、歌词与「律动」、刘海播放器。',
}
