package netease

import (
	"fmt"
	"net/url"
	"strconv"
)

// SubscribePodcast subscribes the listener to a podcast, or unsubscribes
// them.
func SubscribePodcast(id int64, subscribe bool) error {
	return subscription("djradio", subscribe, map[string]any{"id": strconv.FormatInt(id, 10)})
}

// FollowArtist follows an artist for the listener, or unfollows them.
// Through the web API: as the desktop client, through eapi, following is
// refused with -460, 检测到您的网络环境存在风险, though unfollowing is not.
func FollowArtist(id int64, follow bool) error {
	path := "/api/artist/unsub"
	if follow {
		path = "/api/artist/sub"
	}
	form := url.Values{"artistId": {strconv.FormatInt(id, 10)}, "artistIds": {fmt.Sprintf("[%d]", id)}}
	return post(path, form, &struct{}{})
}

func subscription(kind string, sub bool, data map[string]any) error {
	path := "/api/" + kind + "/unsub"
	if sub {
		path = "/api/" + kind + "/sub"
	}
	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := eapiPost(path, data, &res); err != nil {
		return err
	}
	if res.Code != 200 {
		return fmt.Errorf("netease: %s: code %d %s", path, res.Code, res.Message)
	}
	return nil
}
