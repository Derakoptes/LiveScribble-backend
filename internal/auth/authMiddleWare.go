package auth

import (
	"inkline/internal/utils"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

func MiddleWare(jwtKey []byte, DB *gorm.DB) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		//get the jwt key
		tokenString := ctx.GetHeader("Authorization")
		if tokenString == "" {
			ctx.JSON(http.StatusUnauthorized, gin.H{
				"message": "Authorization token is missing",
			})
			ctx.Abort()
			return
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(
			tokenString,
			claims,
			func(t *jwt.Token) (interface{}, error) {
				return jwtKey, nil
			},
		)
		//throw error for invalid token or error
		if err != nil || !token.Valid {
			ctx.JSON(http.StatusUnauthorized, gin.H{
				"message": "Invalid Authorization Token",
			})
			ctx.Abort()
		}
		var user utils.User
		err = DB.Where("id = ?", strings.ToLower(claims.ID)).First(&user).Error
		if err != nil {
			ctx.JSON(
				http.StatusUnauthorized, gin.H{
					"message": "Invalid Authorization Token",
				})
			ctx.Abort()
			return
		}

		ctx.Set("current_user", user.ID)
		ctx.Next()
	}
}
