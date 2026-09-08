package model

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatSession_CreateAndQuery(t *testing.T) {
	truncateTables(t)
	if !ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	s := &ChatSession{TokenId: 1, UserId: 2, ModelName: "glm-5.3", PrefixHash: "a", MessageCount: 3, System: `"sys"`}
	require.NoError(t, s.Insert())
	require.NoError(t, (&ChatTurn{SessionId: s.Id, TurnIndex: 0, RequestId: "r1", NewMessages: `[{"role":"user","content":"hi"}]`, ResponseBody: `{}`}).Insert())
	require.NoError(t, (&ChatTurn{SessionId: s.Id, TurnIndex: 1, RequestId: "r2", NewMessages: `[{"role":"user","content":"again"}]`, ResponseBody: `{}`}).Insert())

	got, err := GetChatSessionById(s.Id)
	require.NoError(t, err)
	assert.Equal(t, 3, got.MessageCount)

	turns, _, err := GetChatTurnsPage(s.Id, 0, 20)
	require.NoError(t, err)
	require.Len(t, turns, 2)
	assert.Equal(t, 0, turns[0].TurnIndex)
	assert.Equal(t, 1, turns[1].TurnIndex)
}

func TestFindChatSessionByPrefixHashes_PrefersLongestMatch(t *testing.T) {
	truncateTables(t)
	if !ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	require.NoError(t, (&ChatSession{TokenId: 1, PrefixHash: "h2", MessageCount: 2}).Insert())
	longer := &ChatSession{TokenId: 1, PrefixHash: "h5", MessageCount: 5}
	require.NoError(t, longer.Insert())
	require.NoError(t, (&ChatSession{TokenId: 2, PrefixHash: "h5", MessageCount: 9}).Insert()) // other token

	got, err := FindChatSessionByPrefixHashes(1, []string{"h2", "h5", "h9"})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 5, got.MessageCount)
}

func TestChatSession_Advance(t *testing.T) {
	truncateTables(t)
	if !ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	s := &ChatSession{TokenId: 1, PrefixHash: "h2", MessageCount: 2}
	require.NoError(t, s.Insert())
	s.MessageCount = 4
	s.PrefixHash = "h4"
	require.NoError(t, s.Advance("glm-5.3", 123))
	got, _ := GetChatSessionById(s.Id)
	assert.Equal(t, 4, got.MessageCount)
	assert.Equal(t, "h4", got.PrefixHash)
	assert.Equal(t, 1, got.TurnCount)
	assert.Equal(t, "glm-5.3", got.ModelName)
	assert.Equal(t, int64(123), got.LastActiveAt)
}

func TestListChatSessions_KeysetPagination(t *testing.T) {
	truncateTables(t)
	if !ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	insert := func(tokenId, userId int, modelName string, at int64) *ChatSession {
		s := &ChatSession{TokenId: tokenId, UserId: userId, ModelName: modelName,
			PrefixHash: fmt.Sprintf("p-%d", at), MessageCount: 1, CreatedAt: at, LastActiveAt: at}
		require.NoError(t, s.Insert())
		return s
	}
	s3 := insert(2, 1, "gpt-4", 300)
	insert(1, 2, "claude", 200)
	s1 := insert(1, 1, "gpt-4", 100)

	// first page, newest first
	list, hasMore, err := ListChatSessions(ChatSessionFilter{}, ChatSessionCursor{}, 2)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.True(t, hasMore)
	assert.Equal(t, int64(300), list[0].LastActiveAt)
	assert.Equal(t, int64(200), list[1].LastActiveAt)

	// walk to the next page with the cursor of the last row
	cursor := ChatSessionCursor{LastActiveAt: list[1].LastActiveAt, Id: list[1].Id}
	list, hasMore, err = ListChatSessions(ChatSessionFilter{}, cursor, 2)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.False(t, hasMore)
	assert.Equal(t, s1.Id, list[0].Id)

	// same last_active_at ties are broken by id descending
	list, _, err = ListChatSessions(ChatSessionFilter{}, ChatSessionCursor{}, 10)
	require.NoError(t, err)
	require.Len(t, list, 3)

	// filters
	list, hasMore, err = ListChatSessions(ChatSessionFilter{TokenId: 1}, ChatSessionCursor{}, 10)
	require.NoError(t, err)
	assert.False(t, hasMore)
	require.Len(t, list, 2)

	list, _, err = ListChatSessions(ChatSessionFilter{UserId: 1}, ChatSessionCursor{}, 10)
	require.NoError(t, err)
	require.Len(t, list, 2)

	list, _, err = ListChatSessions(ChatSessionFilter{ModelName: "gpt-4"}, ChatSessionCursor{}, 10)
	require.NoError(t, err)
	require.Len(t, list, 2)

	// time range on last_active_at, inclusive
	list, _, err = ListChatSessions(ChatSessionFilter{StartTs: 200, EndTs: 300}, ChatSessionCursor{}, 10)
	require.NoError(t, err)
	require.Len(t, list, 2)

	// cursor combined with a filter
	cursor = ChatSessionCursor{LastActiveAt: 300, Id: s3.Id}
	list, _, err = ListChatSessions(ChatSessionFilter{UserId: 1}, cursor, 10)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, s1.Id, list[0].Id)
}

func TestGetChatTurnsPage(t *testing.T) {
	truncateTables(t)
	if !ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	s := &ChatSession{TokenId: 1, PrefixHash: "p"}
	require.NoError(t, s.Insert())
	ids := make([]int, 0, 5)
	for i := 0; i < 5; i++ {
		turn := &ChatTurn{SessionId: s.Id, TurnIndex: i, RequestId: "r", ResponseBody: "{}"}
		require.NoError(t, turn.Insert())
		ids = append(ids, turn.Id)
	}

	// newest page, ascending order inside the page
	turns, hasMore, err := GetChatTurnsPage(s.Id, 0, 3)
	require.NoError(t, err)
	assert.True(t, hasMore)
	require.Len(t, turns, 3)
	assert.Equal(t, ids[2], turns[0].Id)
	assert.Equal(t, ids[4], turns[2].Id)

	// page older than the newest turn id
	turns, hasMore, err = GetChatTurnsPage(s.Id, ids[2], 3)
	require.NoError(t, err)
	assert.False(t, hasMore)
	require.Len(t, turns, 2)
	assert.Equal(t, ids[0], turns[0].Id)

	// other sessions stay invisible
	other := &ChatSession{TokenId: 2, PrefixHash: "q"}
	require.NoError(t, other.Insert())
	require.NoError(t, (&ChatTurn{SessionId: other.Id, TurnIndex: 0}).Insert())
	turns, _, err = GetChatTurnsPage(s.Id, 0, 10)
	require.NoError(t, err)
	assert.Len(t, turns, 5)
}

func TestChatSessionCursor_Roundtrip(t *testing.T) {
	c := ChatSessionCursor{LastActiveAt: 1694123456, Id: 42}
	decoded, err := DecodeChatSessionCursor(c.Encode())
	require.NoError(t, err)
	assert.Equal(t, c, decoded)

	decoded, err = DecodeChatSessionCursor("")
	require.NoError(t, err)
	assert.Equal(t, ChatSessionCursor{}, decoded)

	_, err = DecodeChatSessionCursor("not-base64!!")
	assert.Error(t, err)

	_, err = DecodeChatSessionCursor("Zm9vYmFy") // "foobar", wrong shape
	assert.Error(t, err)
}

// setupChatLogHotCache installs a hot cache for the test and restores the
// previous global state afterwards.
func setupChatLogHotCache(t *testing.T, windowSize, maxTurnBytes int, refreshInterval time.Duration) *chatLogHotCache {
	t.Helper()
	previous := chatLogHot
	cache := newChatLogHotCache(windowSize, maxTurnBytes, refreshInterval)
	chatLogHot = cache
	t.Cleanup(func() {
		chatLogHot = previous
	})
	return cache
}

func TestChatLogHotCache_ListRecentServesWindowAndRefreshes(t *testing.T) {
	truncateTables(t)
	if !ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	cache := setupChatLogHotCache(t, 3, 64<<20, time.Hour)

	s1 := &ChatSession{TokenId: 1, PrefixHash: "p1", LastActiveAt: 100}
	require.NoError(t, s1.Insert()) // write-through populates the window
	s2 := &ChatSession{TokenId: 2, PrefixHash: "p2", LastActiveAt: 200}
	require.NoError(t, s2.Insert())

	sessions, hasMore, ok := ListRecentChatSessions(10)
	require.True(t, ok)
	assert.False(t, hasMore, "window not full, all sessions fit")
	require.Len(t, sessions, 2)
	assert.Equal(t, s2.Id, sessions[0].Id, "newest first")

	// a session written by another node appears after the stale window is refreshed
	s3 := &ChatSession{TokenId: 3, PrefixHash: "p3", LastActiveAt: 300}
	require.NoError(t, CHATLOG_DB.Create(s3).Error) // bypasses write-through
	_, _, ok = ListRecentChatSessions(10)
	require.True(t, ok)
	cache.mu.Lock()
	cache.lastRefresh = time.Now().Add(-2 * time.Hour)
	cache.mu.Unlock()
	sessions, _, ok = ListRecentChatSessions(10)
	require.True(t, ok)
	require.Len(t, sessions, 3)
	assert.Equal(t, s3.Id, sessions[0].Id)

	// the window (not full) holds more than one page: has_more must be true
	sessions, hasMore, ok = ListRecentChatSessions(2)
	require.True(t, ok)
	require.Len(t, sessions, 2)
	assert.True(t, hasMore, "window holds more than one page even when not full")

	// advancing a cached session moves it back to the head with fresh metadata
	s1.TurnCount = 6
	s1.LastActiveAt = 400
	require.NoError(t, s1.Advance("glm-5.3", 400))
	sessions, _, ok = ListRecentChatSessions(10)
	require.True(t, ok)
	assert.Equal(t, s1.Id, sessions[0].Id)
	assert.Equal(t, 7, sessions[0].TurnCount)
}

func TestChatLogHotCache_ListRecentWindowFullReportsMore(t *testing.T) {
	truncateTables(t)
	if !ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	setupChatLogHotCache(t, 2, 64<<20, time.Hour)

	for i := int64(1); i <= 3; i++ {
		require.NoError(t, (&ChatSession{TokenId: int(i), PrefixHash: "p", LastActiveAt: i}).Insert())
	}

	sessions, hasMore, ok := ListRecentChatSessions(2)
	require.True(t, ok)
	assert.True(t, hasMore, "window of 2 is full, older sessions may exist")
	require.Len(t, sessions, 2)
}

func TestChatLogHotCache_TurnsServedFromCache(t *testing.T) {
	truncateTables(t)
	if !ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	setupChatLogHotCache(t, 100, 64<<20, time.Hour)

	s := &ChatSession{TokenId: 1, PrefixHash: "p"}
	require.NoError(t, s.Insert())
	var turns []*ChatTurn
	for i := 0; i < 5; i++ {
		turn := &ChatTurn{SessionId: s.Id, TurnIndex: i, NewMessages: "[]", ResponseBody: "{}"}
		require.NoError(t, turn.Insert())
		turns = append(turns, turn)
	}

	// newest 3 of 5 cached turns
	got, hasMore, ok := GetHotChatTurns(s.Id, 5, 3)
	require.True(t, ok)
	assert.True(t, hasMore)
	require.Len(t, got, 3)
	assert.Equal(t, turns[2].Id, got[0].Id)
	assert.Equal(t, turns[4].Id, got[2].Id)

	// full session in cache, complete view
	got, hasMore, ok = GetHotChatTurns(s.Id, 5, 5)
	require.True(t, ok)
	assert.False(t, hasMore)
	require.Len(t, got, 5)

	// cache holds fewer turns than requested and fewer than exist: cold start
	_, _, ok = GetHotChatTurns(s.Id, 8, 10)
	assert.False(t, ok)

	// cache covers the page size but is behind the DB total (another node
	// appended turns): serving the "newest" page would be stale — cold start
	_, _, ok = GetHotChatTurns(s.Id, 8, 3)
	assert.False(t, ok)

	// a session the cache never saw: cold start
	_, _, ok = GetHotChatTurns(999999, 1, 1)
	assert.False(t, ok)
}

func TestChatLogHotCache_ColdLoadAdmitsTurns(t *testing.T) {
	truncateTables(t)
	if !ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	setupChatLogHotCache(t, 100, 64<<20, time.Hour)

	s := &ChatSession{TokenId: 1, PrefixHash: "p", TurnCount: 2}
	require.NoError(t, s.Insert())
	t1 := &ChatTurn{SessionId: s.Id, TurnIndex: 0, NewMessages: "[]", ResponseBody: "{}"}
	t2 := &ChatTurn{SessionId: s.Id, TurnIndex: 1, NewMessages: "[]", ResponseBody: "{}"}
	require.NoError(t, CHATLOG_DB.Create(t1).Error) // written by another node
	require.NoError(t, CHATLOG_DB.Create(t2).Error)

	// cold load path: DB page then admit, anchored to the DB turn count
	page, hasMore, err := GetChatTurnsPage(s.Id, 0, 20)
	require.NoError(t, err)
	assert.False(t, hasMore)
	AdmitChatTurns(s.Id, page, 2)

	got, hasMore, ok := GetHotChatTurns(s.Id, 2, 20)
	require.True(t, ok)
	assert.False(t, hasMore)
	require.Len(t, got, 2)
}

// TestChatLogHotCache_OversizedNewestTurnKeepsSessionCold pins the fix for
// the PR review finding that admitting a page which silently skips an
// oversized turn could later serve an incomplete "newest page" from cache.
func TestChatLogHotCache_OversizedNewestTurnKeepsSessionCold(t *testing.T) {
	truncateTables(t)
	if !ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	setupChatLogHotCache(t, 100, 64<<20, time.Hour)

	s := &ChatSession{TokenId: 1, PrefixHash: "p", TurnCount: 3}
	require.NoError(t, s.Insert())
	small1 := &ChatTurn{SessionId: s.Id, TurnIndex: 0, NewMessages: "[]", ResponseBody: "{}"}
	require.NoError(t, CHATLOG_DB.Create(small1).Error)
	small2 := &ChatTurn{SessionId: s.Id, TurnIndex: 1, NewMessages: "[]", ResponseBody: "{}"}
	require.NoError(t, CHATLOG_DB.Create(small2).Error)
	big := &ChatTurn{SessionId: s.Id, TurnIndex: 2, NewMessages: "[]", ResponseBody: strings.Repeat("x", maxCachedChatTurnBodyBytes+1)}
	require.NoError(t, CHATLOG_DB.Create(big).Error)

	// cold view loads the newest page; the oversized newest turn makes the
	// page uncacheable as a whole
	page, hasMore, err := GetChatTurnsPage(s.Id, 0, 3)
	require.NoError(t, err)
	assert.False(t, hasMore)
	require.Len(t, page, 3)
	AdmitChatTurns(s.Id, page, 3)

	// every follow-up view must cold-start instead of serving a newest page
	// that silently omits the oversized turn
	_, _, ok := GetHotChatTurns(s.Id, 3, 2)
	assert.False(t, ok, "entry must not claim to hold the newest turns")
	_, _, ok = GetHotChatTurns(s.Id, 3, 3)
	assert.False(t, ok)

	// a write-through oversized turn on an admitted entry also stays cold:
	// covered only grows for cached turns, so it lags the DB count
	fresh := &ChatSession{TokenId: 2, PrefixHash: "q", TurnCount: 1}
	require.NoError(t, fresh.Insert())
	first := &ChatTurn{SessionId: fresh.Id, TurnIndex: 0, NewMessages: "[]", ResponseBody: "{}"}
	require.NoError(t, first.Insert())
	_, _, ok = GetHotChatTurns(fresh.Id, 1, 1)
	require.True(t, ok)
	oversized := &ChatTurn{SessionId: fresh.Id, TurnIndex: 1, NewMessages: "[]", ResponseBody: strings.Repeat("x", maxCachedChatTurnBodyBytes+1)}
	require.NoError(t, CHATLOG_DB.Create(oversized).Error)
	_, _, ok = GetHotChatTurns(fresh.Id, 2, 1)
	assert.False(t, ok, "covered lags the DB count after the oversized append")
}

// the PR review finding that a strictly count-based staleness check disabled
// cache hits for long sessions: a cold-admitted newest page must serve
// follow-up views while the DB turn count is unchanged.
func TestChatLogHotCache_LongSessionNewestPageServesFromCache(t *testing.T) {
	truncateTables(t)
	if !ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	setupChatLogHotCache(t, 100, 64<<20, time.Hour)

	s := &ChatSession{TokenId: 1, PrefixHash: "p", TurnCount: 10}
	require.NoError(t, s.Insert())
	ids := make([]int, 0, 10)
	for i := 0; i < 10; i++ {
		turn := &ChatTurn{SessionId: s.Id, TurnIndex: i, NewMessages: "[]", ResponseBody: "{}"}
		require.NoError(t, CHATLOG_DB.Create(turn).Error) // all written by another node
		ids = append(ids, turn.Id)
	}

	// first view: cold start, only the newest page is loaded and admitted
	page, hasMore, err := GetChatTurnsPage(s.Id, 0, 3)
	require.NoError(t, err)
	assert.True(t, hasMore)
	AdmitChatTurns(s.Id, page, 10)

	// second view of the same page must hit the cache, not the DB
	got, hasMore, ok := GetHotChatTurns(s.Id, 10, 3)
	require.True(t, ok)
	assert.True(t, hasMore, "older turns exist beyond the admitted page")
	require.Len(t, got, 3)
	assert.Equal(t, ids[7], got[0].Id)
	assert.Equal(t, ids[9], got[2].Id)

	// another node appends a turn: the anchored count no longer matches, cold start
	t11 := &ChatTurn{SessionId: s.Id, TurnIndex: 10, NewMessages: "[]", ResponseBody: "{}"}
	require.NoError(t, CHATLOG_DB.Create(t11).Error)
	_, _, ok = GetHotChatTurns(s.Id, 11, 3)
	assert.False(t, ok, "stale entry after another node appended a turn")

	// a write-through append on this node keeps an admitted entry current
	AdmitChatTurns(s.Id, page, 10)
	t12 := &ChatTurn{SessionId: s.Id, TurnIndex: 11, NewMessages: "[]", ResponseBody: "{}"}
	require.NoError(t, t12.Insert())
	got, hasMore, ok = GetHotChatTurns(s.Id, 11, 3)
	require.True(t, ok)
	assert.True(t, hasMore)
	require.Len(t, got, 3)
	assert.Equal(t, t12.Id, got[2].Id, "local append is served as the newest turn")
}

func TestChatLogHotCache_ByteBudgetEvictsLRU(t *testing.T) {
	truncateTables(t)
	if !ChatLogDBEnabled() {
		t.Skip("CHATLOG_DB not configured")
	}
	setupChatLogHotCache(t, 100, 50, time.Hour) // tiny budget: three 20-byte turns overflow it

	mk := func(tokenId int) *ChatSession {
		s := &ChatSession{TokenId: tokenId, PrefixHash: "p"}
		require.NoError(t, s.Insert())
		return s
	}
	s1 := mk(1)
	s2 := mk(2)
	big := &ChatTurn{SessionId: s1.Id, TurnIndex: 0, NewMessages: "0123456789", ResponseBody: "0123456789"} // 20 bytes
	require.NoError(t, big.Insert())
	first := &ChatTurn{SessionId: s2.Id, TurnIndex: 0, NewMessages: "0123456789", ResponseBody: "0123456789"}
	require.NoError(t, first.Insert())

	_, _, ok := GetHotChatTurns(s1.Id, 1, 1)
	require.True(t, ok, "both small sessions fit")

	// touching s2 makes s1 the least recently used; a new turn in s2 evicts s1
	_, _, ok = GetHotChatTurns(s2.Id, 1, 1)
	require.True(t, ok)
	second := &ChatTurn{SessionId: s2.Id, TurnIndex: 1, NewMessages: "0123456789", ResponseBody: "0123456789"}
	require.NoError(t, second.Insert())

	_, _, ok = GetHotChatTurns(s1.Id, 1, 1)
	assert.False(t, ok, "s1 evicted by byte budget")
	_, _, ok = GetHotChatTurns(s2.Id, 2, 2)
	require.True(t, ok, "most recently used session kept")
}

func TestChatLogHotCache_Disabled(t *testing.T) {
	previous := chatLogHot
	chatLogHot = nil
	t.Cleanup(func() { chatLogHot = previous })

	_, _, ok := ListRecentChatSessions(10)
	assert.False(t, ok)
	_, _, ok = GetHotChatTurns(1, 1, 1)
	assert.False(t, ok)
}
