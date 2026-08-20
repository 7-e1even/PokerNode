package wechat

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestAuthenticateLoadsWeChatAvatar(t *testing.T) {
	client := NewClient("app-id", "app-secret", &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		var body string
		switch request.URL.Path {
		case "/sns/oauth2/access_token":
			body = `{"access_token":"access-token","openid":"open-alice"}`
		case "/sns/userinfo":
			if request.URL.Query().Get("openid") != "open-alice" {
				t.Fatalf("profile request openid is %q", request.URL.Query().Get("openid"))
			}
			body = `{"openid":"open-alice","unionid":"union-alice","nickname":"微信 Alice","headimgurl":"http://thirdwx.qlogo.cn/avatar/132"}`
		default:
			t.Fatalf("unexpected WeChat endpoint %q", request.URL.String())
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})})

	profile, err := client.Authenticate(t.Context(), "wechat-code")
	if err != nil {
		t.Fatal(err)
	}
	if profile.OpenID != "open-alice" || profile.UnionID != "union-alice" || profile.Nickname != "微信 Alice" || profile.AvatarURL != "https://thirdwx.qlogo.cn/avatar/132" {
		t.Fatalf("unexpected profile: %#v", profile)
	}
}
