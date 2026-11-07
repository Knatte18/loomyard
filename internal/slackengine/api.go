// api.go is the Slack Web API client: the calls the relay and `lym slack` make, and the failure kinds it tells apart.

package slackengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the Slack Web API's base URL; a method name is appended to it.
const DefaultBaseURL = "https://slack.com/api/"

const (
	// requestTimeout bounds one request to Slack.
	requestTimeout = 20 * time.Second
	// historyPageLimit is how many messages one conversations.history request asks for.
	historyPageLimit = 200
	// defaultRetryAfter is the wait a rate-limited answer with no usable Retry-After header reads as.
	defaultRetryAfter = time.Second
	// rateLimitedCode is the error code Slack answers a rate-limited call with.
	rateLimitedCode = "ratelimited"
)

// API calls Slack's Web API with one bot token.
type API struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewAPI returns an API that posts to baseURL, which ends in a slash, with token as its bearer token, over client.
func NewAPI(baseURL, token string, client *http.Client) *API {
	return &API{baseURL: baseURL, token: token, client: client}
}

// Message is one message of a channel's history.
type Message struct {
	// TS is the message's timestamp, which is also its id in the channel.
	TS string `json:"ts"`
	// ThreadTS is the timestamp of the thread the message belongs to, empty outside a thread.
	ThreadTS string `json:"thread_ts"`
	// User is the Slack user id of the author.
	User string `json:"user"`
	// Subtype is set for a message that is not a plain post, such as a join or an edit.
	Subtype string `json:"subtype"`
	// BotID is set for a message a bot wrote.
	BotID string `json:"bot_id"`
	// Text is the message text as Slack delivers it.
	Text string `json:"text"`
}

// ChannelInfo is what conversations.info tells about a channel.
type ChannelInfo struct {
	// ID is the channel's Slack id.
	ID string `json:"id"`
	// Name is the channel's name.
	Name string `json:"name"`
	// IsMember reports whether the bot is a member of the channel.
	IsMember bool `json:"is_member"`
	// IsPrivate reports whether the channel is private.
	IsPrivate bool `json:"is_private"`
}

// APIError is a call Slack answered with ok false and an error code.
// Codes include `msg_too_long`, `invalid_auth` and `channel_not_found`.
type APIError struct {
	// Method is the Web API method that was called.
	Method string
	// Code is the error code Slack answered.
	Code string
}

// Error returns the method and Slack's error code.
func (e *APIError) Error() string {
	return fmt.Sprintf("slack: %s refused: %s", e.Method, e.Code)
}

// RateLimitError is a call Slack refused for its rate limit.
type RateLimitError struct {
	// Method is the Web API method that was called.
	Method string
	// RetryAfter is how long Slack asks the caller to wait before the next call.
	RetryAfter time.Duration
}

// Error returns the method and the wait Slack asked for.
func (e *RateLimitError) Error() string {
	return fmt.Sprintf("slack: %s rate limited; retry after %s", e.Method, e.RetryAfter)
}

// response is the part of a Slack answer the client reads, whatever the method.
type response struct {
	OK               bool          `json:"ok"`
	Error            string        `json:"error"`
	Messages         []Message     `json:"messages"`
	HasMore          bool          `json:"has_more"`
	Channel          ChannelInfo   `json:"channel"`
	ResponseMetadata responseCurse `json:"response_metadata"`
}

// responseCurse holds the cursor of the next page of a paginated answer.
type responseCurse struct {
	NextCursor string `json:"next_cursor"`
}

// call posts form to method and decodes Slack's answer.
// An HTTP 429 and an ok-false `ratelimited` are a *RateLimitError.
// Any other ok false is an *APIError, and a transport failure, a 5xx or an unreadable answer is a plain error.
// No error carries the token.
func (a *API) call(ctx context.Context, method string, form url.Values) (response, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+method, strings.NewReader(form.Encode()))
	if err != nil {
		return response{}, fmt.Errorf("slack: build the %s request: %w", method, err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "Bearer "+a.token)
	httpResponse, err := a.client.Do(request)
	if err != nil {
		return response{}, fmt.Errorf("slack: call %s: %w", method, err)
	}
	defer httpResponse.Body.Close()
	switch {
	case httpResponse.StatusCode == http.StatusTooManyRequests:
		return response{}, &RateLimitError{Method: method, RetryAfter: retryAfter(httpResponse)}
	case httpResponse.StatusCode >= http.StatusInternalServerError:
		return response{}, fmt.Errorf("slack: %s answered HTTP %d", method, httpResponse.StatusCode)
	}
	body, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return response{}, fmt.Errorf("slack: read the %s answer: %w", method, err)
	}
	var decoded response
	if err := json.Unmarshal(body, &decoded); err != nil {
		return response{}, fmt.Errorf("slack: decode the %s answer: %w", method, err)
	}
	if decoded.OK {
		return decoded, nil
	}
	if decoded.Error == rateLimitedCode {
		return response{}, &RateLimitError{Method: method, RetryAfter: retryAfter(httpResponse)}
	}
	return response{}, &APIError{Method: method, Code: decoded.Error}
}

// retryAfter reads the Retry-After header of httpResponse in seconds;
// a missing or unreadable header reads as defaultRetryAfter.
func retryAfter(httpResponse *http.Response) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(httpResponse.Header.Get("Retry-After")))
	if err != nil || seconds < 1 {
		return defaultRetryAfter
	}
	return time.Duration(seconds) * time.Second
}

// AuthTest checks the token with auth.test.
// A refused token is an *APIError.
func (a *API) AuthTest(ctx context.Context) error {
	_, err := a.call(ctx, "auth.test", url.Values{})
	return err
}

// ConversationInfo reads the channel with conversations.info.
func (a *API) ConversationInfo(ctx context.Context, channel string) (ChannelInfo, error) {
	answer, err := a.call(ctx, "conversations.info", url.Values{"channel": {channel}})
	if err != nil {
		return ChannelInfo{}, err
	}
	return answer.Channel, nil
}

// UserInfo checks with users.info that the Slack user exists.
func (a *API) UserInfo(ctx context.Context, user string) error {
	_, err := a.call(ctx, "users.info", url.Values{"user": {user}})
	return err
}

// History reads every message of the channel posted after oldest, which is exclusive, oldest first.
// It follows Slack's pagination until the channel has no more.
func (a *API) History(ctx context.Context, channel, oldest string) ([]Message, error) {
	var messages []Message
	cursor := ""
	for {
		form := url.Values{"channel": {channel}, "oldest": {oldest}, "limit": {strconv.Itoa(historyPageLimit)}}
		if cursor != "" {
			form.Set("cursor", cursor)
		}
		page, err := a.call(ctx, "conversations.history", form)
		if err != nil {
			return nil, err
		}
		messages = append(messages, page.Messages...)
		cursor = page.ResponseMetadata.NextCursor
		if !page.HasMore || cursor == "" {
			break
		}
	}
	sort.SliceStable(messages, func(i, j int) bool { return compareTS(messages[i].TS, messages[j].TS) < 0 })
	return messages, nil
}

// Latest returns the timestamp of the channel's newest message, and reports found false for an empty channel.
func (a *API) Latest(ctx context.Context, channel string) (string, bool, error) {
	page, err := a.call(ctx, "conversations.history", url.Values{"channel": {channel}, "limit": {"1"}})
	if err != nil {
		return "", false, err
	}
	if len(page.Messages) == 0 {
		return "", false, nil
	}
	return page.Messages[0].TS, true, nil
}

// PostMessage posts text to the channel with chat.postMessage.
func (a *API) PostMessage(ctx context.Context, channel, text string) error {
	_, err := a.call(ctx, "chat.postMessage", url.Values{"channel": {channel}, "text": {text}})
	return err
}

// compareTS orders two Slack timestamps, which read `<seconds>.<micros>`, oldest first.
func compareTS(a, b string) int {
	secondsA, microsA, _ := strings.Cut(a, ".")
	secondsB, microsB, _ := strings.Cut(b, ".")
	if len(secondsA) != len(secondsB) {
		return len(secondsA) - len(secondsB)
	}
	if byLength := strings.Compare(secondsA, secondsB); byLength != 0 {
		return byLength
	}
	return strings.Compare(microsA, microsB)
}

// isAPIError reports whether err is or wraps an *APIError, and returns it.
func isAPIError(err error) (*APIError, bool) {
	var apiErr *APIError
	return apiErr, errors.As(err, &apiErr)
}

// isRateLimit reports whether err is or wraps a *RateLimitError, and returns it.
func isRateLimit(err error) (*RateLimitError, bool) {
	var limit *RateLimitError
	return limit, errors.As(err, &limit)
}
