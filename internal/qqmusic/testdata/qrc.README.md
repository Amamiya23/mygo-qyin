# QRC protocol regression samples

Captured from anonymous POST requests to QQ Music's
`https://u.y.qq.com/cgi-bin/musicu.fcg` during the 2026-11-03 session.
These files contain only the returned encrypted lyric fields, no cookies,
account IDs, tokens or response headers.

Request: `music.musichallSong.PlayLyricInfo/GetPlayLyricInfo`,
`comm={ct:24,cv:0,uin:"0",format:"json"}`,
`param={songMID:<below>,crypt:1,ct:24,cv:0,lver:0,qrc:1,trans:1,roma:0}`.
The Qingtian request used `roma:1`; its romanization and translation were empty.

| File | Song MID | Field | Observed response |
| --- | --- | --- | --- |
| qrc_qingtian.hex | 0039MnYb0qxYhV | lyric | qrc_t=1761231326; 9808 hex characters |
| qrc_kimi.hex | 002pXw073bnbSj | lyric | qrc_t=1719592142; 4336 hex characters |
| qrc_kimi_trans.hex | 002pXw073bnbSj | trans | 1648 hex characters; decoded translation is LRC |
| qrc_lrc_only.hex | 003XGaNw3g8rjL | lyric | qrc_t=0, lrc_t=1763630554; 176 hex characters |

All four samples pass full zlib decompression and checksum verification after
QQ DES-EDE decryption. They exercise the real byte order, altered S2/S4 entries,
and the D-half PC-2 selection offset, which the supplied interface report did
not describe. No tolerant/truncated zlib path is needed for these samples.

These fixtures preserve wire data for offline regression tests; tests do not
request the network or depend on the upstream lyrics remaining unchanged.

