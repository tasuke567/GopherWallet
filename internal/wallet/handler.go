package wallet

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v2"

	"github.com/wey/gopher-wallet/internal/domain"
)

type Handler struct {
	transferSvc *TransferService
	accountRepo domain.AccountRepository
	logger      *slog.Logger
}

func NewHandler(transferSvc *TransferService, accountRepo domain.AccountRepository, logger *slog.Logger) *Handler {
	return &Handler{
		transferSvc: transferSvc,
		accountRepo: accountRepo,
		logger:      logger,
	}
}

func (h *Handler) RegisterRoutes(app *fiber.App) {
	api := app.Group("/api/v1")
	api.Post("/accounts", h.CreateAccount)
	api.Get("/accounts/:id", h.GetAccount)
	api.Post("/transfers", h.Transfer)
}

// CreateAccount godoc
// POST /api/v1/accounts
func (h *Handler) CreateAccount(c *fiber.Ctx) error {
	type request struct {
		UserID   string `json:"user_id"`
		Balance  int64  `json:"balance"`
		Currency string `json:"currency"`
	}

	var req request
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.UserID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "user_id is required"})
	}
	if req.Currency == "" {
		req.Currency = "THB"
	}

	account := &domain.Account{
		UserID:   req.UserID,
		Balance:  req.Balance,
		Currency: req.Currency,
	}

	if err := h.accountRepo.Create(c.UserContext(), account); err != nil {
		h.logger.Error("failed to create account", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create account"})
	}

	return c.Status(fiber.StatusCreated).JSON(account)
}

// GetAccount godoc
// GET /api/v1/accounts/:id
func (h *Handler) GetAccount(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid account id"})
	}

	account, err := h.accountRepo.GetByID(c.UserContext(), int64(id))
	if err != nil {
		if errors.Is(err, domain.ErrAccountNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "account not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
	}

	return c.JSON(account)
}

// Transfer godoc
// POST /api/v1/transfers
func (h *Handler) Transfer(c *fiber.Ctx) error {
	var req TransferRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	// Use Idempotency-Key header if not in body
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = c.Get("Idempotency-Key")
	}
	if req.IdempotencyKey == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "idempotency_key is required"})
	}

	result, err := h.transferSvc.Transfer(c.UserContext(), req)
	if err != nil {
		return h.handleTransferError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(result)
}

func (h *Handler) handleTransferError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, domain.ErrSameAccount):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, domain.ErrInvalidAmount):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, domain.ErrAccountNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, domain.ErrInsufficientBalance):
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, domain.ErrCurrencyMismatch):
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, domain.ErrDuplicateTransaction):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
	}
}
