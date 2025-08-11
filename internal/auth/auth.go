package auth

import (
	"encoding/json"
	"errors"
	"livescribble/internal/utils"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type Handler struct {
	db     *gorm.DB
	jwtKey []byte
	logger *slog.Logger
}
type Claims struct {
	ID string `json:"id"`
	jwt.RegisteredClaims
}

func NewHandler(db *gorm.DB, jwtKey []byte, logger *slog.Logger) *Handler {
	return &Handler{
		db:     db,
		jwtKey: jwtKey,
		logger: logger,
	}
}

type Request struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Login(ctx *gin.Context) {
	var req Request
	err := json.Unmarshal([]byte(ctx.PostForm("request")), &req)
	if err != nil {
		ctx.JSON(
			http.StatusBadRequest,
			gin.H{"message": "Missing Request"},
		)
		return
	}

	var user utils.User
	err = h.db.Where("email = ?", req.Email).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			ctx.JSON(
				http.StatusBadRequest,
				gin.H{"message": "User not found"},
			)
		} else {
			ctx.JSON(
				http.StatusInternalServerError,
				gin.H{"message": "Internal server error"},
			)
		}
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(req.Password), []byte(user.Password))
	if err != nil {
		ctx.JSON(
			http.StatusBadRequest,
			gin.H{"message": "Incorrect password"},
		)
		return
	}

	//Password and Email Correct
	token, err := createToken(user.ID, h.jwtKey)
	if err != nil {
		h.logger.Error("Failed to create token", err)
		ctx.JSON(
			http.StatusInternalServerError,
			gin.H{"message": "Server error"},
		)
		return
	}
	ctx.JSON(
		http.StatusOK,
		gin.H{"token": token},
	)
}

func (h *Handler) Register(ctx *gin.Context) {
	var req Request
	err := json.Unmarshal([]byte(ctx.PostForm("request")), &req)
	if err != nil {
		ctx.JSON(
			http.StatusBadRequest,
			gin.H{"message": "Missing Request"},
		)
		return
	}

	var checkUser utils.User
	err = h.db.Where("email = ?", req.Email).First(&checkUser).Error
	if err == nil {
		ctx.JSON(
			http.StatusBadRequest,
			gin.H{"message": "User already exists"},
		)
		return
	}
	//user doesnt exist, create
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		h.logger.Error("Failed to hash password", err)
		ctx.JSON(
			http.StatusInternalServerError,
			gin.H{"message": "Server error"},
		)
		return
	}
	userID, err := generateUserID()
	if err != nil {
		h.logger.Error("Failed to generate user id", err)
		ctx.JSON(
			http.StatusInternalServerError,
			gin.H{"message": "Server error"},
		)
		return
	}
	newUser := utils.User{
		ID:       userID,
		Email:    req.Email,
		Password: string(hashedPassword),
	}
	err = h.db.Create(&newUser).Error
	if err != nil {
		h.logger.Error("Failed to save user", err)
		ctx.JSON(
			http.StatusInternalServerError,
			gin.H{"message": "Server error"},
		)
		return
	}
	token, err := createToken(newUser.ID, h.jwtKey)
	if err != nil {
		h.logger.Error("Failed to generate access token", err)
		ctx.JSON(
			http.StatusInternalServerError,
			gin.H{"message": "Server error"},
		)
		return
	}
	ctx.JSON(
		http.StatusOK,
		gin.H{"token": token},
	)
}

func createToken(id string, secretKey []byte) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		ID: id,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 24 * 7)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	tokenString, err := token.SignedString(secretKey)
	if err != nil {
		return "", err
	}
	return tokenString, nil
}

func generateUserID() (string, error) {
	return utils.RandomString(16)
}
