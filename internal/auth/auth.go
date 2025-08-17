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
	token, err := createToken(user.ID, h.jwtKey, 7)
	if err != nil {
		h.logger.Error("Failed to create token", "error", err.Error())
		ctx.JSON(
			http.StatusInternalServerError,
			gin.H{"message": "Server error"},
		)
		return
	}

	h.setCookie(ctx, token, 7*24*time.Hour)
	ctx.JSON(
		http.StatusOK,
		gin.H{"message": "Login successful"},
	)
}

func (h *Handler) CreateTempUser(ctx *gin.Context) {
	var req Request
	err := json.Unmarshal([]byte(ctx.PostForm("request")), &req)
	if err != nil {
		ctx.JSON(
			http.StatusBadRequest,
			gin.H{"message": "Missing Request"},
		)
		return
	}

	tempID, err := generateTempID()
	if err != nil {
		h.logger.Error("Failed to generate temp user id", "error", err)
		ctx.JSON(
			http.StatusInternalServerError,
			gin.H{"message": "Server error"},
		)
		return
	}

	// Add "temp" prefix to the ID
	tempUserID := "temp" + tempID

	// Hash the password before storing
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		h.logger.Error("Failed to hash password", "error", err)
		ctx.JSON(
			http.StatusInternalServerError,
			gin.H{"message": "Server error"},
		)
		return
	}

	// Set deleted_at to 1 day from now
	deletedAt := time.Now().Add(24 * time.Hour)

	tempUser := utils.User{
		ID:        tempUserID,
		Email:     tempUserID, // Just set this to avoid unique email constraint
		Password:  string(hashedPassword),
		DeletedAt: deletedAt,
	}

	err = h.db.Create(&tempUser).Error
	if err != nil {
		h.logger.Error("Failed to create temp user", "error", err.Error())
		ctx.JSON(
			http.StatusInternalServerError,
			gin.H{"message": "Server error"},
		)
		return
	}

	// Create token that lasts 1 day
	token, err := createToken(tempUser.ID, h.jwtKey, 1)
	if err != nil {
		h.logger.Error("Failed to create token for temp user", "error", err.Error())
		ctx.JSON(
			http.StatusInternalServerError,
			gin.H{"message": "Server error"},
		)
		return
	}

	h.setCookie(ctx, token, 24*time.Hour)
	ctx.JSON(
		http.StatusOK,
		gin.H{
			"message": "Temp user created",
			"tempId":  tempUser.ID,
		},
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
		h.logger.Error("Failed to hash password", "error", err)
		ctx.JSON(
			http.StatusInternalServerError,
			gin.H{"message": "Server error"},
		)
		return
	}
	userID, err := generateUserID()
	if err != nil {
		h.logger.Error("Failed to generate user id", "error", err)
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
		h.logger.Error("Failed to save user", "error", err.Error())
		ctx.JSON(
			http.StatusInternalServerError,
			gin.H{"message": "Server error"},
		)
		return
	}
	token, err := createToken(newUser.ID, h.jwtKey, 7)
	if err != nil {
		h.logger.Error("Failed to generate access token ", "error", err.Error())
		ctx.JSON(
			http.StatusInternalServerError,
			gin.H{"message": "Server error"},
		)
		return
	}

	h.setCookie(ctx, token, 7*24*time.Hour)
	ctx.JSON(
		http.StatusOK,
		gin.H{"message": "Registration successful"},
	)
}

func createToken(id string, secretKey []byte, duration int) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		ID: id,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 24 * time.Duration(duration))),
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
	return utils.RandomString(10)
}

func generateTempID() (string, error) {
	return utils.RandomString(6)
}

func (h *Handler) setCookie(ctx *gin.Context, token string, maxAge time.Duration) {
	ctx.SetCookie(
		"auth_token",          
		token,                 
		int(maxAge.Seconds()), 
		"/",                   
		"",                    
		true,                 
		true,                 	
	)
}
