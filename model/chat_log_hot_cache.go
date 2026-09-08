package model

import (
	"container/list"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// chatLogHotCache keeps recent chat-log data in memory so the admin views of
// active conversations do not hit the (potentially huge) chat-log database:
//
//   - sessions: a newest-first window of recent session metadata, refreshed
//     from the DB every refreshInterval and updated write-through on every
//     session insert/advance done by this node.
//   - turns: per-session turn lists (including response bodies) under a
//     process-wide byte budget, LRU-evicted at session granularity. Turns are
//     immutable once written, so cached turns are always safe to serve.
//
// Anything outside these structures is cold data: it is loaded from the DB on
// first access (and the newest page of a cold session is admitted to the
// cache until budget pressure evicts it).
type chatLogHotCache struct {
	mu sync.Mutex

	windowSize      int
	maxTurnBytes    int
	refreshInterval time.Duration

	sessions     []*ChatSession       // newest-first, metadata only
	sessionsById map[int]*ChatSession // id -> entry shared with sessions
	windowFull   bool                 // window holds windowSize entries: older rows may exist in the DB
	lastRefresh  time.Time

	turns          map[int]*chatTurnCacheEntry // session id -> entry
	lru            *list.List                  // session ids, front = most recently used
	lruElems       map[int]*list.Element
	totalTurnBytes int
}

// chatTurnCacheEntry holds the newest turns of one session. covered is the
// DB turn count the entry is synced to: when covered equals the DB-fresh
// TurnCount, the entry's turns are the session's newest. turns may hold
// fewer rows than covered when oversized turns were skipped on admission —
// such entries only serve pages fully inside the cached range.
type chatTurnCacheEntry struct {
	turns   []*ChatTurn // ascending by turn id
	covered int
	bytes   int
}

// maxCachedChatTurnBodyBytes caps admission of a single turn's bodies; larger
// turns stay DB-only instead of pinning oversized buffers in memory.
const maxCachedChatTurnBodyBytes = 1 << 20

var chatLogHot *chatLogHotCache

func InitChatLogHotCache() {
	if !common.GetEnvOrDefaultBool("CHAT_LOG_HOT_CACHE_ENABLED", true) || !ChatLogDBEnabled() {
		chatLogHot = nil
		return
	}
	chatLogHot = newChatLogHotCache(
		common.GetEnvOrDefault("CHAT_LOG_HOT_SESSION_WINDOW", 1000),
		common.GetEnvOrDefault("CHAT_LOG_HOT_TURN_BYTES", 64<<20),
		time.Second*time.Duration(common.GetEnvOrDefault("CHAT_LOG_HOT_REFRESH_SECONDS", 60)),
	)
}

func newChatLogHotCache(windowSize, maxTurnBytes int, refreshInterval time.Duration) *chatLogHotCache {
	if windowSize < 1 {
		windowSize = 1
	}
	if maxTurnBytes < 1 {
		maxTurnBytes = 1
	}
	if refreshInterval < time.Second {
		refreshInterval = time.Second
	}
	return &chatLogHotCache{
		windowSize:      windowSize,
		maxTurnBytes:    maxTurnBytes,
		refreshInterval: refreshInterval,
		sessionsById:    make(map[int]*ChatSession),
		turns:           make(map[int]*chatTurnCacheEntry),
		lru:             list.New(),
		lruElems:        make(map[int]*list.Element),
	}
}

func (c *chatLogHotCache) recordSession(s *ChatSession) {
	if s == nil || s.Id == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.upsertSessionLocked(s)
}

func (c *chatLogHotCache) upsertSessionLocked(s *ChatSession) {
	if existing, ok := c.sessionsById[s.Id]; ok {
		// refresh goroutines and controllers read cached copies after the lock
		// is released; mutate the shared entry only under the lock
		*existing = *s
		if existing != c.sessions[0] {
			c.moveSessionToFrontLocked(s.Id)
		}
		return
	}
	// copy: the caller's session object is owned by the persist goroutine
	entry := *s
	c.sessionsById[entry.Id] = &entry
	c.sessions = append([]*ChatSession{&entry}, c.sessions...)
	c.trimSessionWindowLocked()
}

func (c *chatLogHotCache) moveSessionToFrontLocked(id int) {
	for i, s := range c.sessions {
		if s.Id == id {
			c.sessions = append(c.sessions[:i], c.sessions[i+1:]...)
			break
		}
	}
	c.sessions = append([]*ChatSession{c.sessionsById[id]}, c.sessions...)
}

func (c *chatLogHotCache) trimSessionWindowLocked() {
	if len(c.sessions) <= c.windowSize {
		return
	}
	for _, s := range c.sessions[c.windowSize:] {
		delete(c.sessionsById, s.Id)
	}
	c.sessions = c.sessions[:c.windowSize]
	c.windowFull = true
}

func (c *chatLogHotCache) recordTurn(t *ChatTurn) {
	if t == nil || t.SessionId == 0 {
		return
	}
	bodyBytes := len(t.NewMessages) + len(t.ResponseBody)
	if bodyBytes > maxCachedChatTurnBodyBytes {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.turns[t.SessionId]
	if entry == nil {
		entry = &chatTurnCacheEntry{}
		c.turns[t.SessionId] = entry
	}
	turns := entry.turns
	i := sort.Search(len(turns), func(i int) bool { return turns[i].Id >= t.Id })
	if i < len(turns) && turns[i].Id == t.Id {
		return // already recorded
	}
	// the cache owns its copies: the caller's struct may be reused after insert
	owned := *t
	turns = append(turns, nil)
	copy(turns[i+1:], turns[i:])
	turns[i] = &owned
	entry.turns = turns
	entry.bytes += bodyBytes
	c.totalTurnBytes += bodyBytes
	// this node just persisted one more turn, so whatever DB total the entry
	// was synced to grew by exactly one; if the entry was already behind
	// (other nodes wrote), the equality check in getTurns still fails safely
	entry.covered++
	c.touchTurnsLocked(t.SessionId)
	c.evictTurnsLocked()
}

func (c *chatLogHotCache) touchTurnsLocked(sessionId int) {
	if elem, ok := c.lruElems[sessionId]; ok {
		c.lru.MoveToFront(elem)
		return
	}
	c.lruElems[sessionId] = c.lru.PushFront(sessionId)
}

func (c *chatLogHotCache) evictTurnsLocked() {
	// evict down to the byte budget; the last session may go too, otherwise a
	// single oversized entry could keep the cache above budget forever
	for c.totalTurnBytes > c.maxTurnBytes && c.lru.Len() > 0 {
		c.evictOldestTurnSessionLocked()
	}
}

func (c *chatLogHotCache) evictOldestTurnSessionLocked() {
	back := c.lru.Back()
	if back == nil {
		return
	}
	sessionId := back.Value.(int)
	c.lru.Remove(back)
	delete(c.lruElems, sessionId)
	c.totalTurnBytes -= c.turns[sessionId].bytes
	delete(c.turns, sessionId)
}

// getTurns serves the newest `limit` turns of a session from cache when the
// entry is provably current: entry.covered must equal the DB-fresh
// totalTurns (otherwise some other node appended turns this cache has not
// seen, and the "newest" page would be stale), and the cached range must
// fully cover the requested page. Returns ok=false when the caller must
// fall back to the database.
func (c *chatLogHotCache) getTurns(sessionId, totalTurns, limit int) (turns []*ChatTurn, hasMore bool, ok bool) {
	if limit < 1 {
		limit = 1
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, exists := c.turns[sessionId]
	if !exists {
		return nil, false, false
	}
	c.touchTurnsLocked(sessionId)
	if entry.covered != totalTurns {
		return nil, false, false // cache is behind the database: cold start
	}
	n := len(entry.turns)
	// the entry may hold fewer turns than covered (oversized turns are never
	// admitted); it can only serve pages fully inside the cached range
	if n < limit && n < entry.covered {
		return nil, false, false
	}
	if n >= limit {
		turns = entry.turns[n-limit:]
	} else {
		turns = entry.turns
	}
	return turns, entry.covered > limit, true
}

// admitTurns stores a cold-loaded newest page. totalTurns is the DB-fresh
// session turn count, which anchors the entry: as long as the count is
// unchanged, the entry is known to hold the session's newest turns — even
// when it holds only the newest page of a long session.
func (c *chatLogHotCache) admitTurns(sessionId int, turns []*ChatTurn, totalTurns int) {
	if sessionId == 0 || len(turns) == 0 || totalTurns < len(turns) {
		return
	}
	bytes := 0
	kept := make([]*ChatTurn, 0, len(turns))
	for _, t := range turns {
		bodyBytes := len(t.NewMessages) + len(t.ResponseBody)
		if bodyBytes > maxCachedChatTurnBodyBytes {
			continue
		}
		// the cache owns its copies: admitted turns are also handed to the
		// HTTP handler for serialization
		owned := *t
		kept = append(kept, &owned)
		bytes += bodyBytes
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.replaceTurnsLocked(sessionId, kept, totalTurns, bytes)
	c.evictTurnsLocked()
}

func (c *chatLogHotCache) replaceTurnsLocked(sessionId int, turns []*ChatTurn, covered int, bytes int) {
	c.touchTurnsLocked(sessionId)
	if old, exists := c.turns[sessionId]; exists {
		c.totalTurnBytes -= old.bytes
	}
	c.turns[sessionId] = &chatTurnCacheEntry{turns: turns, covered: covered, bytes: bytes}
	c.totalTurnBytes += bytes
}

// listRecent serves the first page of the unfiltered, newest-first session
// list from the in-memory window. The window is refreshed from the DB when
// stale (covers multi-node deployments and the cold start after boot).
// Returns ok=false when the hot cache is disabled or the refresh failed.
func (c *chatLogHotCache) listRecent(limit int) (sessions []*ChatSession, hasMore bool, ok bool) {
	if limit < 1 {
		limit = 1
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.lastRefresh) >= c.refreshInterval {
		if err := c.refreshLocked(); err != nil {
			return nil, false, false
		}
	}
	n := len(c.sessions)
	if n == 0 {
		return nil, c.windowFull, true
	}
	if n > limit {
		sessions = c.sessions[:limit]
	} else {
		sessions = c.sessions
	}
	// return copies: cached entries are mutated by later advance calls under
	// the lock, readers must not share the structs
	out := make([]*ChatSession, len(sessions))
	for i, s := range sessions {
		copyS := *s
		out[i] = &copyS
	}
	// hasMore must compare the pre-slice window length: the window may hold
	// more entries than this page even when it is not full
	return out, n > limit || c.windowFull, true
}

func (c *chatLogHotCache) refreshLocked() error {
	var sessions []*ChatSession
	if err := CHATLOG_DB.Order("last_active_at desc, id desc").Limit(c.windowSize).Find(&sessions).Error; err != nil {
		return err
	}
	c.sessions = sessions
	c.sessionsById = make(map[int]*ChatSession, len(sessions))
	for _, s := range sessions {
		c.sessionsById[s.Id] = s
	}
	c.windowFull = len(sessions) == c.windowSize
	c.lastRefresh = time.Now()
	return nil
}

func chatLogCacheRecordSession(s *ChatSession) {
	if chatLogHot != nil {
		chatLogHot.recordSession(s)
	}
}

func chatLogCacheAdvanceSession(s *ChatSession) {
	if chatLogHot != nil {
		chatLogHot.recordSession(s)
	}
}

func chatLogCacheRecordTurn(t *ChatTurn) {
	if chatLogHot != nil {
		chatLogHot.recordTurn(t)
	}
}

// ListRecentChatSessions serves the first page of the newest-first session
// list from the hot cache. ok=false means the caller should query the DB.
func ListRecentChatSessions(limit int) (sessions []*ChatSession, hasMore bool, ok bool) {
	if chatLogHot == nil {
		return nil, false, false
	}
	return chatLogHot.listRecent(limit)
}

// GetHotChatTurns serves a session's newest turns from the hot cache.
// ok=false means the caller should query the DB (cold start).
func GetHotChatTurns(sessionId, totalTurns, limit int) (turns []*ChatTurn, hasMore bool, ok bool) {
	if chatLogHot == nil {
		return nil, false, false
	}
	return chatLogHot.getTurns(sessionId, totalTurns, limit)
}

// AdmitChatTurns stores a cold-loaded session page in the hot cache so
// follow-up views of the same session are served from memory. totalTurns is
// the DB-fresh session turn count that anchors the entry's currency.
func AdmitChatTurns(sessionId int, turns []*ChatTurn, totalTurns int) {
	if chatLogHot != nil {
		chatLogHot.admitTurns(sessionId, turns, totalTurns)
	}
}
