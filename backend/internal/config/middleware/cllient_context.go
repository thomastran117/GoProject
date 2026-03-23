package middleware

import (
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

type DeviceType string

const (
	DeviceUnknown DeviceType = "unknown"
	DeviceBot     DeviceType = "bot"
	DeviceMobile  DeviceType = "mobile"
	DeviceTablet  DeviceType = "tablet"
	DeviceDesktop DeviceType = "desktop"
)

type ClientInfo struct {
	UserAgent  string
	DeviceType DeviceType
	IsMobile   bool
	IsTablet   bool
	IsBot      bool
}

const clientInfoKey = "client_info"

var (
	botPattern    = regexp.MustCompile(`(?i)bot|crawl|spider|slurp|mediapartners|apis-google|feedfetcher|lighthouse`)
	mobilePattern = regexp.MustCompile(`(?i)Mobile|iPhone|iPod|Android.*Mobile|Windows Phone|BlackBerry|Opera Mini|IEMobile`)
	tabletPattern = regexp.MustCompile(`(?i)iPad|Tablet|Kindle|Silk|PlayBook`)
)

func ClientInfoMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ua := sanitizeUserAgent(c.GetHeader("User-Agent"))
		deviceType := resolveDeviceType(ua)

		info := ClientInfo{
			UserAgent:  ua,
			DeviceType: deviceType,
			IsMobile:   deviceType == DeviceMobile,
			IsTablet:   deviceType == DeviceTablet,
			IsBot:      deviceType == DeviceBot,
		}

		c.Set(clientInfoKey, info)
		c.Next()
	}
}

func sanitizeUserAgent(ua string) string {
	ua = strings.TrimSpace(ua)
	if ua == "" {
		return ""
	}

	const maxUALength = 512
	if len(ua) > maxUALength {
		return ua[:maxUALength]
	}

	return ua
}

func resolveDeviceType(ua string) DeviceType {
	if ua == "" {
		return DeviceUnknown
	}

	if botPattern.MatchString(ua) {
		return DeviceBot
	}

	if strings.Contains(ua, "Android") && !strings.Contains(ua, "Mobile") {
		return DeviceTablet
	}

	if tabletPattern.MatchString(ua) {
		return DeviceTablet
	}

	if mobilePattern.MatchString(ua) {
		return DeviceMobile
	}

	return DeviceDesktop
}

func GetClientInfo(c *gin.Context) (ClientInfo, bool) {
	value, exists := c.Get(clientInfoKey)
	if !exists {
		return ClientInfo{}, false
	}

	info, ok := value.(ClientInfo)
	return info, ok
}