package domain

import (
	"fmt"
	"strings"
)

// Version labels (spec RF-24, §5 <versión>): the label that tells apart the versions of one
// place (a movie, or the chapter of a series), read from the video and audio tracks with ffprobe.
// The rule is pinned by contracts/version-labels.json, shared with the web app. Pure: no I/O.

// VersionMedia is what a version label reads from a video: its main video track and its audio
// tracks, as ffprobe reports them.
type VersionMedia struct {
	Width, Height int
	// Codec is the codec name of the main video track, as ffprobe writes it ("hevc").
	Codec string
	Audio []AudioMedia
}

// AudioMedia is one audio track: its language tag and its title, as ffprobe reports them.
type AudioMedia struct {
	Language string
	Title    string
}

// VersionLabel is the proposed label of one version. Incomplete is true when the rule could not
// name a part that is what tells the versions apart (an audio track with no language, a codec
// without a short name, a frame without a class): the label then leaves that part out and the
// user completes it in the preview (RF-24), it is never invented.
type VersionLabel struct {
	Text       string
	Incomplete bool
}

// resolutionClass is a class of resolution: its label and the frame it stands for.
type resolutionClass struct {
	label         string
	width, height int
}

// resolutionClasses are tried from the largest. A video takes the first class whose frame it
// reaches, by its width or by its height (see resolutionReach): a letterboxed 1920×800 is 1080p and
// a 1280×534 is 720p, and a 4:3 720×576 is 576p.
var resolutionClasses = []resolutionClass{
	{label: "2160p", width: 3840, height: 2160},
	{label: "1440p", width: 2560, height: 1440},
	{label: "1080p", width: 1920, height: 1080},
	{label: "720p", width: 1280, height: 720},
	{label: "576p", width: 1024, height: 576},
	{label: "480p", width: 854, height: 480},
}

// resolutionReach is the share of a class's frame, in percent, that a width or a height must reach.
const resolutionReach = 98

// codecLabels are the short names of the codecs (RF-24): HEVC, AV1 and H.264 as the spec writes them.
var codecLabels = map[string]string{
	"hevc":       "HEVC",
	"h264":       "H.264",
	"av1":        "AV1",
	"vp9":        "VP9",
	"mpeg2video": "MPEG-2",
	"vc1":        "VC-1",
}

// languageLabels are the names, in Spanish, of the audio languages that have one (RF-24). Spanish
// has its own rule (languageOf).
var languageLabels = map[string]string{
	"eng": "Inglés", "en": "Inglés",
	"jpn": "Japonés", "ja": "Japonés",
	"fra": "Francés", "fre": "Francés", "fr": "Francés",
	"deu": "Alemán", "ger": "Alemán", "de": "Alemán",
	"ita": "Italiano", "it": "Italiano",
	"por": "Portugués", "pt": "Portugués",
	"kor": "Coreano", "ko": "Coreano",
	"zho": "Chino", "chi": "Chino", "zh": "Chino",
	"rus": "Ruso", "ru": "Ruso",
}

// spanishLanguages are the language tags of Spanish. Any es-<region> tag is Spanish too.
var spanishLanguages = map[string]bool{"spa": true, "es": true, "esp": true}

// castellanoTitles are the words of a Spanish track's title that make it Castellano (RF-24).
var castellanoTitles = []string{"castellano", "españa", "espana", "es-es"}

// VersionLabels returns the label of each version of one place, in the order given (RF-24):
//
//   - the resolution of its frame, always;
//   - the codec, when some versions share the resolution and their codecs differ;
//   - the audio, when some versions share the resolution and the codec and their audio differs:
//     Latino for a Spanish track without more indication, Castellano when its title says so, the
//     name in Spanish of another language, and Dual for two or more languages;
//   - a label that is still equal to an earlier one gets " (2)", " (3)"… from the second on.
//
// A part that the rule cannot name, and that is what tells the versions apart, is left out and
// marks the label Incomplete: the frame with no class (or no frame size), a codec without a short
// name, an audio with no track or with a track whose language has no name here.
func VersionLabels(versions []VersionMedia) []VersionLabel {
	n := len(versions)
	text := make([]string, n)
	incomplete := make([]bool, n)
	resolutions := make([]string, n)
	for i, v := range versions {
		r, ok := resolutionOf(v.Width, v.Height)
		if !ok {
			incomplete[i] = true
		}
		resolutions[i] = r
	}
	members := make([]int, n)
	for i := range members {
		members[i] = i
	}
	labelGroup(members, versions, resolutions, text, incomplete)
	return withSuffixes(text, incomplete)
}

// labelGroup labels the versions in members: by resolution, then by codec within a resolution,
// then by audio within a codec. It fills text, and marks incomplete the versions whose missing
// part was needed.
func labelGroup(members []int, versions []VersionMedia, resolutions, text []string, incomplete []bool) {
	codecKey := func(i int) string { return versions[i].Codec }
	for _, sameRes := range partition(members, func(i int) string { return resolutions[i] }) {
		byCodec := partition(sameRes, codecKey)
		codecsDiffer := len(byCodec) > 1
		for _, sameCodec := range byCodec {
			first := sameCodec[0]
			head := resolutions[first]
			if codecsDiffer {
				if codec, ok := codecLabels[versions[first].Codec]; ok {
					head = joinParts(head, codec)
				} else {
					for _, i := range sameCodec {
						incomplete[i] = true
					}
				}
			}
			if len(sameCodec) == 1 {
				text[first] = head
				continue
			}
			tokens := make([]string, len(sameCodec))
			for k, i := range sameCodec {
				token, ok := audioLabel(versions[i].Audio)
				if !ok {
					incomplete[i] = true
				}
				tokens[k] = token
			}
			differ := false
			for _, t := range tokens {
				if t != tokens[0] {
					differ = true
				}
			}
			for k, i := range sameCodec {
				text[i] = head
				if differ {
					text[i] = joinParts(head, tokens[k])
				}
			}
		}
	}
}

// joinParts joins the parts of a label that are present, with a space.
func joinParts(head, token string) string {
	switch {
	case head == "":
		return token
	case token == "":
		return head
	default:
		return head + " " + token
	}
}

// audioLabel names the audio of one version: the language of its tracks, or Dual when they are
// two or more languages. The same language counts once, and Latino and Castellano are two. It
// reports false when there is no track or a track whose language has no name here.
func audioLabel(tracks []AudioMedia) (string, bool) {
	if len(tracks) == 0 {
		return "", false
	}
	var names []string
	seen := map[string]bool{}
	for _, a := range tracks {
		name, ok := languageOf(a)
		if !ok {
			return "", false
		}
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	if len(names) > 1 {
		return "Dual", true
	}
	return names[0], true
}

// languageOf names one audio track's language. A Spanish track is Castellano when its title says
// so, and Latino otherwise (RF-24). It reports false for a language that has no name here.
func languageOf(a AudioMedia) (string, bool) {
	lang := strings.ToLower(a.Language)
	if spanishLanguages[lang] || strings.HasPrefix(lang, "es-") {
		title := strings.ToLower(a.Title)
		for _, word := range castellanoTitles {
			if strings.Contains(title, word) {
				return "Castellano", true
			}
		}
		return "Latino", true
	}
	name, ok := languageLabels[lang]
	return name, ok
}

// resolutionOf returns the label of a frame of width × height, or false when it has no class.
func resolutionOf(width, height int) (string, bool) {
	if width <= 0 || height <= 0 {
		return "", false
	}
	for _, c := range resolutionClasses {
		if width*100 >= c.width*resolutionReach || height*100 >= c.height*resolutionReach {
			return c.label, true
		}
	}
	return "", false
}

// partition splits members into groups by key, the groups in the order of their first member.
func partition(members []int, key func(int) string) [][]int {
	var order []string
	groups := map[string][]int{}
	for _, i := range members {
		k := key(i)
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], i)
	}
	out := make([][]int, 0, len(order))
	for _, k := range order {
		out = append(out, groups[k])
	}
	return out
}

// withSuffixes makes equal labels distinct: the first keeps its label, the next ones get " (2)", " (3)"…
func withSuffixes(text []string, incomplete []bool) []VersionLabel {
	count := map[string]int{}
	out := make([]VersionLabel, len(text))
	for i, l := range text {
		count[l]++
		out[i] = VersionLabel{Text: l, Incomplete: incomplete[i]}
		if n := count[l]; n > 1 {
			out[i].Text = fmt.Sprintf("%s (%d)", l, n)
		}
	}
	return out
}
