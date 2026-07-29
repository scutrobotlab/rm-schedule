package handler

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/kataras/iris/v12"
	"github.com/sirupsen/logrus"
)

const (
	envIsTestEnvironment      = "SCHEDULE_IS_TEST_ENVIRONMENT"
	envMobileBracketRollout   = "SCHEDULE_MOBILE_BRACKET_ROLLOUT"
	envExperimentCookieSecret = "SCHEDULE_EXPERIMENT_COOKIE_SECRET"

	bracketUICookieName     = "bracket_ui_v1"
	defaultExperimentSecret = "rm-schedule-bracket-ui-default-secret-v1"
	bracketBucketCount      = 10000
	bracketCookieMaxAge     = 400 * 24 * 60 * 60
)

var (
	defaultSecretLogOnce  sync.Once
	generateBracketBucket = randomBracketBucket
)

type globalConfig struct {
	IsTestEnvironment    bool `json:"isTestEnvironment"`
	MobileBracketEnabled bool `json:"mobileBracketEnabled"`
}

func isTestEnvironment() bool {
	value, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(envIsTestEnvironment)))
	return err == nil && value
}

func mobileBracketRolloutThreshold() int {
	raw := strings.TrimSpace(os.Getenv(envMobileBracketRollout))
	if raw == "" {
		return 0
	}

	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 100 {
		logrus.Errorf("%s must be a number between 0 and 100; using 0", envMobileBracketRollout)
		return 0
	}
	return int(math.Round(value * float64(bracketBucketCount) / 100))
}

func experimentCookieSecret() []byte {
	value := strings.TrimSpace(os.Getenv(envExperimentCookieSecret))
	if value == "" {
		defaultSecretLogOnce.Do(func() {
			logrus.Errorf("%s is not set; using insecure built-in default", envExperimentCookieSecret)
		})
		value = defaultExperimentSecret
	}
	return []byte(value)
}

func randomBracketBucket() (int, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(bracketBucketCount))
	if err != nil {
		return 0, err
	}
	return int(value.Int64()), nil
}

func bracketBucketSignature(bucket int, secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(strconv.Itoa(bucket)))
	return mac.Sum(nil)
}

func encodeBracketCookie(bucket int, secret []byte) string {
	return fmt.Sprintf("%d.%s", bucket, hex.EncodeToString(bracketBucketSignature(bucket, secret)))
}

func decodeBracketCookie(value string, secret []byte) (int, bool) {
	bucketText, signatureText, found := strings.Cut(value, ".")
	if !found || strings.Contains(signatureText, ".") {
		return 0, false
	}

	bucket, err := strconv.Atoi(bucketText)
	if err != nil || bucket < 0 || bucket >= bracketBucketCount {
		return 0, false
	}

	providedSignature, err := hex.DecodeString(signatureText)
	if err != nil {
		return 0, false
	}
	expectedSignature := bracketBucketSignature(bucket, secret)
	if !hmac.Equal(providedSignature, expectedSignature) {
		return 0, false
	}
	return bucket, true
}

func requestIsHTTPS(c iris.Context) bool {
	return c.Request().TLS != nil ||
		strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")), "https")
}

func resolveBracketBucket(c iris.Context, secret []byte) (int, bool) {
	if cookie, err := c.Request().Cookie(bracketUICookieName); err == nil {
		if bucket, valid := decodeBracketCookie(cookie.Value, secret); valid {
			return bucket, true
		}
	}

	bucket, err := generateBracketBucket()
	if err != nil {
		logrus.Errorf("generate Bracket UI experiment bucket failed: %v", err)
		return 0, false
	}
	c.SetCookie(&http.Cookie{
		Name:     bracketUICookieName,
		Value:    encodeBracketCookie(bucket, secret),
		Path:     "/",
		MaxAge:   bracketCookieMaxAge,
		HttpOnly: true,
		Secure:   requestIsHTTPS(c),
		SameSite: http.SameSiteLaxMode,
	})
	return bucket, true
}

// ConfigHandler 下发由运行时环境变量决定的前端全局配置。
func ConfigHandler(c iris.Context) {
	c.Header("Cache-Control", "no-store")
	bucket, available := resolveBracketBucket(c, experimentCookieSecret())
	threshold := mobileBracketRolloutThreshold()
	c.JSON(globalConfig{
		IsTestEnvironment:    isTestEnvironment(),
		MobileBracketEnabled: available && bucket < threshold,
	})
}
