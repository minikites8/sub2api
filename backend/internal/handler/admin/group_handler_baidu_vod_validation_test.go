package admin

import (
	"testing"

	"github.com/gin-gonic/gin/binding"
	"github.com/stretchr/testify/require"
)

func TestGroupRequestValidationAcceptsBaiduVODPlatform(t *testing.T) {
	createReq := CreateGroupRequest{Name: "baidu-vod-default", Platform: "baidu_vod"}
	require.NoError(t, binding.Validator.ValidateStruct(createReq))

	updateReq := UpdateGroupRequest{Platform: "baidu_vod"}
	require.NoError(t, binding.Validator.ValidateStruct(updateReq))
}
