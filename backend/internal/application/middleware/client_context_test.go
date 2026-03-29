package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newClientInfoRouter() *gin.Engine {
	r := gin.New()
	r.GET("/", ClientInfoMiddleware(), func(c *gin.Context) {
		info, ok := GetClientInfo(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "missing"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"device_type": string(info.DeviceType),
			"is_mobile":   info.IsMobile,
			"is_tablet":   info.IsTablet,
			"is_bot":      info.IsBot,
		})
	})
	return r
}

func clientInfoFor(t *testing.T, ua string) map[string]any {
	t.Helper()
	r := newClientInfoRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", w.Code, w.Body.String())
	}
	return responseBody(t, w)
}

// --- resolveDeviceType via middleware ---

func TestClientInfo_DesktopUA(t *testing.T) {
	info := clientInfoFor(t, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	if info["device_type"] != string(DeviceDesktop) {
		t.Errorf("expected desktop, got %v", info["device_type"])
	}
	if info["is_mobile"] != false {
		t.Error("expected IsMobile=false for desktop UA")
	}
}

func TestClientInfo_MobileUA_iPhone(t *testing.T) {
	info := clientInfoFor(t, "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) Mobile/15E148")
	if info["device_type"] != string(DeviceMobile) {
		t.Errorf("expected mobile, got %v", info["device_type"])
	}
	if info["is_mobile"] != true {
		t.Error("expected IsMobile=true")
	}
}

func TestClientInfo_TabletUA_iPad(t *testing.T) {
	info := clientInfoFor(t, "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/537.36")
	if info["device_type"] != string(DeviceTablet) {
		t.Errorf("expected tablet, got %v", info["device_type"])
	}
	if info["is_tablet"] != true {
		t.Error("expected IsTablet=true")
	}
}

func TestClientInfo_AndroidTablet(t *testing.T) {
	// Android without "Mobile" in the UA → tablet
	info := clientInfoFor(t, "Mozilla/5.0 (Linux; Android 13; SM-T870) AppleWebKit/537.36")
	if info["device_type"] != string(DeviceTablet) {
		t.Errorf("expected tablet for Android non-mobile UA, got %v", info["device_type"])
	}
}

func TestClientInfo_AndroidMobile(t *testing.T) {
	info := clientInfoFor(t, "Mozilla/5.0 (Linux; Android 13; Pixel 7) Mobile AppleWebKit/537.36")
	if info["device_type"] != string(DeviceMobile) {
		t.Errorf("expected mobile for Android Mobile UA, got %v", info["device_type"])
	}
}

func TestClientInfo_BotUA(t *testing.T) {
	info := clientInfoFor(t, "Googlebot/2.1 (+http://www.google.com/bot.html)")
	if info["device_type"] != string(DeviceBot) {
		t.Errorf("expected bot, got %v", info["device_type"])
	}
	if info["is_bot"] != true {
		t.Error("expected IsBot=true")
	}
}

func TestClientInfo_EmptyUA_ReturnsUnknown(t *testing.T) {
	info := clientInfoFor(t, "")
	if info["device_type"] != string(DeviceUnknown) {
		t.Errorf("expected unknown for empty UA, got %v", info["device_type"])
	}
}

// --- sanitizeUserAgent ---

func TestSanitizeUserAgent_TruncatesLongUA(t *testing.T) {
	long := strings.Repeat("a", 600)
	result := sanitizeUserAgent(long)
	if len(result) != 512 {
		t.Errorf("expected UA truncated to 512, got %d", len(result))
	}
}

func TestSanitizeUserAgent_TrimsWhitespace(t *testing.T) {
	result := sanitizeUserAgent("  Mozilla/5.0  ")
	if result != "Mozilla/5.0" {
		t.Errorf("expected trimmed UA, got %q", result)
	}
}

func TestSanitizeUserAgent_EmptyReturnsEmpty(t *testing.T) {
	if got := sanitizeUserAgent(""); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// --- GetClientInfo ---

func TestGetClientInfo_WithoutMiddleware_ReturnsFalse(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	_, ok := GetClientInfo(c)
	if ok {
		t.Error("expected ok=false when middleware was not applied")
	}
}
