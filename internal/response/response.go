package response

import "github.com/gin-gonic/gin"

// Body adalah format respons JSON standar seluruh endpoint.
type Body struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Success mengirim respons status "success" (HTTP 2xx).
func Success(c *gin.Context, code int, message string, data interface{}) {
	c.JSON(code, Body{Status: "success", Message: message, Data: data})
}

// Fail mengirim respons status "fail" (HTTP 4xx, kesalahan klien).
func Fail(c *gin.Context, code int, message string) {
	c.JSON(code, Body{Status: "fail", Message: message})
}

// Error mengirim respons status "error" (HTTP 5xx, kesalahan server).
func Error(c *gin.Context, code int, message string) {
	c.JSON(code, Body{Status: "error", Message: message})
}

// AbortFail seperti Fail tetapi menghentikan rantai middleware.
func AbortFail(c *gin.Context, code int, message string) {
	c.AbortWithStatusJSON(code, Body{Status: "fail", Message: message})
}

// AbortError seperti Error tetapi menghentikan rantai middleware.
func AbortError(c *gin.Context, code int, message string) {
	c.AbortWithStatusJSON(code, Body{Status: "error", Message: message})
}
