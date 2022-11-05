//
// Copyright (c) 2019 Ted Unangst <tedu@tedunangst.com>
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that the above
// copyright notice and this permission notice appear in all copies.
//
// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
// WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
// MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
// ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
// WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
// ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
// OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

package main

import (
	"regexp"

	"github.com/mattn/go-runewidth"
)

var bigboldshitz = "𝐀𝐁𝐂𝐃𝐄𝐅𝐆𝐇𝐈𝐉𝐊𝐋𝐌𝐍𝐎𝐏𝐐𝐑𝐒𝐓𝐔𝐕𝐖𝐗𝐘𝐙"
var lilboldshitz = "𝐚𝐛𝐜𝐝𝐞𝐟𝐠𝐡𝐢𝐣𝐤𝐥𝐦𝐧𝐨𝐩𝐪𝐫𝐬𝐭𝐮𝐯𝐰𝐱𝐲𝐳"
var moeboldshitz = "𝗔𝗕𝗖𝗗𝗘𝗙𝗚𝗛𝗜𝗝𝗞𝗟𝗠𝗡𝗢𝗣𝗤𝗥𝗦𝗧𝗨𝗩𝗪𝗫𝗬𝗭"
var morboldshitz = "𝗮𝗯𝗰𝗱𝗲𝗳𝗴𝗵𝗶𝗷𝗸𝗹𝗺𝗻𝗼𝗽𝗾𝗿𝘀𝘁𝘂𝘃𝘄𝘅𝘆𝘇"
var biggothshitz = "𝕬𝕭𝕮𝕯𝕰𝕱𝕲𝕳𝕴𝕵𝕶𝕷𝕸𝕹𝕺𝕻𝕼𝕽𝕾𝕿𝖀𝖁𝖂𝖃𝖄𝖅"
var lilgothshitz = "𝖆𝖇𝖈𝖉𝖊𝖋𝖌𝖍𝖎𝖏𝖐𝖑𝖒𝖓𝖔𝖕𝖖𝖗𝖘𝖙𝖚𝖛𝖜𝖝𝖞𝖟"
var moegothshitz = "𝔄𝔅𝕮𝔇𝔈𝔉𝔊𝕳ℑ𝔍𝔎𝔏𝔐𝔑𝔒𝔓𝔔ℜ𝔖𝔗𝔘𝔙𝔚𝔛𝔜𝖅"
var morgothshitz = "𝔞𝔟𝔠𝔡𝔢𝔣𝔤𝔥𝔦𝔧𝔨𝔩𝔪𝔫𝔬𝔭𝔮𝔯𝔰𝔱𝔲𝔳𝔴𝔵𝔶𝔷"
var bigitalshitz = "𝑨𝑩𝑪𝑫𝑬𝑭𝑮𝑯𝑰𝑱𝑲𝑳𝑴𝑵𝑶𝑷𝑸𝑹𝑺𝑻𝑼𝑽𝑾𝑿𝒀𝒁"
var lilitalshitz = "𝒂𝒃𝒄𝒅𝒆𝒇𝒈𝒉𝒊𝒋𝒌𝒍𝒎𝒏𝒐𝒑𝒒𝒓𝒔𝒕𝒖𝒗𝒘𝒙𝒚𝒛"
var moeitalshitz = "𝘼𝘽𝘾𝘿𝙀𝙁𝙂𝙃𝙄𝙅𝙆𝙇𝙈𝙉𝙊𝙋𝙌𝙍𝙎𝙏𝙐𝙑𝙒𝙓𝙔𝙕"
var moritalshitz = "𝙖𝙗𝙘𝙙𝙚𝙛𝙜𝙝𝙞𝙟𝙠𝙡𝙢𝙣𝙤𝙥𝙦𝙧𝙨𝙩𝙪𝙫𝙬𝙭𝙮𝙯"
var bigbangshitz = "𝔸𝔹ℂ𝔻𝔼𝔽𝔾ℍ𝕀𝕁𝕂𝕃𝕄ℕ𝕆ℙℚℝ𝕊𝕋𝕌𝕍𝕎𝕏𝕐ℤ"
var lilbangshitz = "𝕒𝕓𝕔𝕕𝕖𝕗𝕘𝕙𝕚𝕛𝕜𝕝𝕞𝕟𝕠𝕡𝕢𝕣𝕤𝕥𝕦𝕧𝕨𝕩𝕪𝕫"
var bigwideshitz = "ＡＢＣＤＥＦＧＨＩＪＫＬＭＮＯＰＱＲＳＴＵＶＷＸＹＺ"
var lilwideshitz = "ａｂｃｄｅｆｇｈｉｊｋｌｍｎｏｐｑｒｓｔｕｖｗｘｙｚ"
var bigblokshitz = "🅰🅱🅲🅳🅴🅵🅶🅷🅸🅹🅺🅻🅼🅽🅾🅿🆀🆁🆂🆃🆄🆅🆆🆇🆈🆉"

var re_alltheshitz = regexp.MustCompile(`([` +
	bigboldshitz + lilboldshitz +
	moeboldshitz + morboldshitz +
	biggothshitz + lilgothshitz +
	moegothshitz + morgothshitz +
	bigitalshitz + lilitalshitz +
	moeitalshitz + moritalshitz +
	bigbangshitz + lilbangshitz +
	bigwideshitz + lilwideshitz +
	bigblokshitz +
	"][ '\ufe0f]?){3,}")

var allUppers = []string{bigboldshitz, moeboldshitz, biggothshitz, bigwideshitz, moegothshitz, bigitalshitz, moeitalshitz, bigbangshitz, bigblokshitz}
var allLowers = []string{lilboldshitz, morboldshitz, lilgothshitz, lilwideshitz, morgothshitz, lilitalshitz, moritalshitz, lilbangshitz}

var re_moredumb = regexp.MustCompile(`[👏]`)

// this may not be especially fast
func unpucker(s string) string {
	fixer := func(r string) string {
		x := make([]byte, len(r))
		xi := 0
	loop1:
		for _, c := range r {
			xi++
			if c == ' ' || c == '\'' {
				x[xi] = byte(c)
				continue
			}
			for _, set := range allUppers {
				i := 0
				for _, rr := range set {
					if rr == c {
						x[xi] = byte('A' + i)
						continue loop1
					}
					i++
				}
			}
			for _, set := range allLowers {
				i := 0
				for _, rr := range set {
					if rr == c {
						x[xi] = byte('a' + i)
						continue loop1
					}
					i++
				}
			}
			x[xi] = '.'
		}
		return string(x)
	}
	s = re_alltheshitz.ReplaceAllStringFunc(s, fixer)

	s = re_moredumb.ReplaceAllString(s, ".")

	zw := false
	for _, c := range s {
		if runewidth.RuneWidth(c) == 0 {
			zw = true
			break
		}
	}
	if zw {
		x := make([]byte, 0, len(s))
		zw = false
		for _, c := range s {
			if runewidth.RuneWidth(c) == 0 {
				if zw {
					continue
				}
				zw = true
			} else {
				zw = false
			}
			q := string(c)
			x = append(x, []byte(q)...)
		}
		return string(x)
	}
	return s
}
