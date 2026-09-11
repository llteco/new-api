package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertUsersForGroupFilterTest(t *testing.T) {
	t.Helper()
	truncateTables(t)
	users := []*User{
		{Username: "single", Password: "password123", Group: "group0", AffCode: "aff-s0"},
		{Username: "multi", Password: "password123", Group: "group0,group1", AffCode: "aff-m0"},
		{Username: "other", Password: "password123", Group: "group10", AffCode: "aff-o0"},
	}
	for _, user := range users {
		require.NoError(t, DB.Create(user).Error)
	}
}

func TestSearchUsersGroupFilterMatchesMultiGroupUsers(t *testing.T) {
	insertUsersForGroupFilterTest(t)

	users, total, err := SearchUsers("", "group0", nil, nil, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.ElementsMatch(t, []string{"single", "multi"}, usernames(users))

	// 按列表中任意一个分组过滤都能命中多分组用户；
	// 同时验证不会用子串误匹配：group1 不命中 group10
	users, total, err = SearchUsers("", "group1", nil, nil, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, users, 1)
	assert.Equal(t, "multi", users[0].Username)

	_, total, err = SearchUsers("", "group10", nil, nil, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
}

func usernames(users []*User) []string {
	names := make([]string, 0, len(users))
	for _, user := range users {
		names = append(names, user.Username)
	}
	return names
}

func TestEditWithTxNormalizesGroupList(t *testing.T) {
	insertUsersForGroupFilterTest(t)
	var multi User
	require.NoError(t, DB.Where("username = ?", "multi").First(&multi).Error)

	multi.Group = " group1 , , group0 , group1 "
	require.NoError(t, multi.EditWithTx(DB, false))
	assert.Equal(t, "group1,group0", multi.Group)

	var reloaded User
	require.NoError(t, DB.Where("username = ?", "multi").First(&reloaded).Error)
	assert.Equal(t, "group1,group0", reloaded.Group)

	// 空分组规范化为 default
	multi.Group = " , "
	require.NoError(t, multi.EditWithTx(DB, false))
	assert.Equal(t, "default", multi.Group)
}
