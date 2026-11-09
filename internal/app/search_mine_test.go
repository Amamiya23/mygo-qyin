package app

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

// Searching the listener's playlists finds the songs with every word of the
// query in their name, alias, artists or album, in the playlists' order,
// each once, ignoring case.
func TestMineMatches(t *testing.T) {
	mygo := []qqmusic.Artist{{Name: "MyGO!!!!!"}}
	liked := &qqmusic.Playlist{Tracks: []qqmusic.Track{
		{ID: 1, Name: "春日影", Artists: mygo, Album: qqmusic.Album{Name: "迷跡波"}},
		{ID: 2, Name: "Ether", Artists: []qqmusic.Artist{{Name: "Ave Mujica"}}},
		{ID: 3, Name: "影色舞", Alias: []string{"Kageiro"}, Artists: mygo},
	}}
	other := &qqmusic.Playlist{Tracks: []qqmusic.Track{
		{ID: 4, Name: "碧天伴走", Artists: mygo, Album: qqmusic.Album{Name: "迷跡波"}},
		{ID: 1, Name: "春日影", Artists: mygo, Album: qqmusic.Album{Name: "迷跡波"}},
	}}
	lists := []*qqmusic.Playlist{liked, other}
	ids := func(q string) []int64 {
		var out []int64
		for _, tr := range mineMatches(lists, q) {
			out = append(out, tr.ID)
		}
		return out
	}
	for _, tc := range []struct {
		q    string
		want []int64
	}{
		{"影", []int64{1, 3}},
		{"mygo 迷跡波", []int64{1, 4}},
		{"KAGEIRO", []int64{3}},
		{"ave ether", []int64{2}},
		{"mygo ether", nil},
		{"  ", nil},
	} {
		if got := ids(tc.q); !slices.Equal(got, tc.want) {
			t.Errorf("%q found %v, want %v", tc.q, got, tc.want)
		}
	}
}

// The 我的歌单 tab loads the listener's playlists not loaded yet, lists the
// songs in them that match, and a click plays those songs.
func TestSearchMine(t *testing.T) {
	asked := make(chan int64, 4)
	mineDetail = func(id int64) (*qqmusic.Playlist, error) {
		asked <- id
		return &qqmusic.Playlist{ID: id}, nil
	}
	searchResults = func(string, qqmusic.SearchType, int, int) (qqmusic.SearchResult, error) {
		return qqmusic.SearchResult{}, nil
	}
	defer func() { mineDetail, searchResults = qqmusic.PlaylistDetail, qqmusic.SearchPage }()

	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &qqmusic.Profile{UserID: 1, Nickname: "灯"}
	a.mine = []qqmusic.Playlist{{ID: 10, Name: "我喜欢", SpecialType: 5}, {ID: 11, Name: "练习曲"}}
	a.playlists[10] = &qqmusic.Playlist{ID: 10, Tracks: []qqmusic.Track{
		{ID: 1, Name: "春日影", Artists: []qqmusic.Artist{{Name: "MyGO!!!!!"}}, Duration: 200000},
		{ID: 2, Name: "Ether", Artists: []qqmusic.Artist{{Name: "Ave Mujica"}}, Duration: 200000},
		{ID: 3, Name: "碧天伴走", Artists: []qqmusic.Artist{{Name: "MyGO!!!!!"}}, Duration: 200000},
	}}
	a.searchFor(route{page: pageSearch, query: "mygo", kind: qqmusic.SearchSongs})
	// The page alone: the sidebar's section of the same name would take the click.
	tt := ui.NewTester(func(c *ui.Context) {
		a.pal = paletteFor(c.Theme())
		ui.Column(c).Fill().Children(func() { a.content(c) })
	}, 1200, 800)
	tt.Frame()

	if err := tt.Click("我的歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if id := wait(t, asked, "the playlist not loaded"); id != 11 {
		t.Fatalf("loaded playlist %d", id)
	}
	// As the playlist lands, which post drops without a window.
	a.playlists[11] = &qqmusic.Playlist{ID: 11, Tracks: []qqmusic.Track{
		{ID: 4, Name: "迷星叫", Artists: []qqmusic.Artist{{Name: "MyGO!!!!!"}}, Duration: 200000},
	}}
	a.loadSearch(a.route, true)
	tt.Frame()
	if len(asked) > 0 {
		t.Error("a loaded playlist was asked for again")
	}

	if err := tt.Click("碧天伴走"); err != nil {
		t.Fatal(err)
	}
	var queue []int64
	for _, tr := range a.pl.Queue() {
		queue = append(queue, tr.ID)
	}
	if cur := a.pl.Current(); cur == nil || cur.ID != 3 || !slices.Equal(queue, []int64{1, 3, 4}) {
		t.Fatalf("playing %+v of %v", cur, queue)
	}
}
