# Real Use Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add real-use readiness features: administrator password changes, user password resets, simple point-rule messaging, failure refund messaging, and 30-day image retention guidance.

**Architecture:** Keep the current Go API and React/Vite structure. Add password helper behavior in the `auth` package, expose two admin-only password endpoints in `admin`, wire routes through the existing app router, and add focused UI state to `AdminPage` while keeping user-facing rule text on `WorkspacePage` and `HistoryPage`.

**Tech Stack:** Go, chi, pgx, bcrypt, PostgreSQL, React, TypeScript, Vitest, Testing Library.

---

## File Structure

- Modify `api/internal/auth/service.go`: add password validation helpers and reuse them from registration, login, admin bootstrap, and password updates.
- Modify `api/internal/auth/service_test.go`: cover password helper behavior and service-level password update behavior.
- Modify `api/internal/admin/handlers.go`: add `ChangeOwnPassword` and `ResetUserPassword` handlers with audit logs.
- Modify `api/internal/admin/handlers_test.go`: add route registration and handler tests for both password flows.
- Modify `api/internal/app/routes.go`: register the new admin endpoints.
- Modify `web/src/pages/AdminPage.tsx`: add the “安全” tab, own-password form, reset-password inline row, and point-rule note.
- Modify `web/src/pages/AdminPage.test.tsx`: test the new tab and reset-password request.
- Modify `web/src/pages/WorkspacePage.tsx`: add concise usage text and fixed failure refund text.
- Modify `web/src/pages/WorkspacePage.test.tsx`: test rule text and fixed failure message.
- Modify `web/src/pages/HistoryPage.tsx`: add 30-day/download reminder text.
- Modify `web/src/pages/HistoryPage.test.tsx`: test retention reminder.
- Modify `README.md`: document the fixed point, refund, and 30-day user-facing rules.

---

### Task 1: Auth Password Helpers

**Files:**
- Modify: `api/internal/auth/service.go`
- Modify: `api/internal/auth/service_test.go`

- [x] **Step 1: Write failing tests for password validation and service password updates**

Add these tests to `api/internal/auth/service_test.go` after `TestLoginRejectsDisabledUser`:

```go
func TestValidateNewPasswordRequiresSixCharacters(t *testing.T) {
	for _, tc := range []struct {
		name     string
		password string
		valid    bool
	}{
		{name: "six characters", password: "123456", valid: true},
		{name: "longer password", password: "secure-password", valid: true},
		{name: "five characters", password: "12345", valid: false},
		{name: "empty", password: "", valid: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateNewPassword(tc.password)
			if tc.valid && err != nil {
				t.Fatalf("ValidateNewPassword(%q) error = %v, want nil", tc.password, err)
			}
			if !tc.valid && !errors.Is(err, ErrPasswordTooShort) {
				t.Fatalf("ValidateNewPassword(%q) error = %v, want ErrPasswordTooShort", tc.password, err)
			}
		})
	}
}

func TestChangePasswordRequiresCurrentPassword(t *testing.T) {
	ctx, db := setupAuthTestDB(t)
	service := Service{DB: db}

	hash, err := bcrypt.GenerateFromPassword([]byte("old-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	var userID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, role, status, credit_balance)
		VALUES ('password-admin', $1, $2, $3, 0)
		RETURNING id::text
	`, string(hash), models.RoleAdmin, models.UserStatusActive).Scan(&userID); err != nil {
		t.Fatalf("insert admin: %v", err)
	}

	if err := service.ChangePassword(ctx, userID, "old-password", "new-password"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if _, err := service.Login(ctx, "password-admin", "new-password"); err != nil {
		t.Fatalf("login with new password: %v", err)
	}
	if _, err := service.Login(ctx, "password-admin", "old-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("login with old password error = %v, want ErrInvalidCredentials", err)
	}
}

func TestChangePasswordRejectsWrongCurrentPassword(t *testing.T) {
	ctx, db := setupAuthTestDB(t)
	service := Service{DB: db}

	hash, err := bcrypt.GenerateFromPassword([]byte("old-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	var userID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, role, status, credit_balance)
		VALUES ('wrong-current-admin', $1, $2, $3, 0)
		RETURNING id::text
	`, string(hash), models.RoleAdmin, models.UserStatusActive).Scan(&userID); err != nil {
		t.Fatalf("insert admin: %v", err)
	}

	err = service.ChangePassword(ctx, userID, "bad-password", "new-password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("ChangePassword error = %v, want ErrInvalidCredentials", err)
	}
	if _, err := service.Login(ctx, "wrong-current-admin", "old-password"); err != nil {
		t.Fatalf("old password should still work: %v", err)
	}
}

func TestResetPasswordUpdatesTargetPassword(t *testing.T) {
	ctx, db := setupAuthTestDB(t)
	service := Service{DB: db}

	hash, err := bcrypt.GenerateFromPassword([]byte("old-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	var userID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, role, status, credit_balance)
		VALUES ('reset-user', $1, $2, $3, 0)
		RETURNING id::text
	`, string(hash), models.RoleUser, models.UserStatusActive).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := service.ResetPassword(ctx, userID, "new-password"); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if _, err := service.Login(ctx, "reset-user", "new-password"); err != nil {
		t.Fatalf("login with reset password: %v", err)
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run:

```bash
cd api && go test ./internal/auth
```

Expected: FAIL with undefined names such as `ValidateNewPassword`, `ErrPasswordTooShort`, `ChangePassword`, and `ResetPassword`.

- [x] **Step 3: Implement password helpers and service methods**

In `api/internal/auth/service.go`, update the error block and add the constant:

```go
var (
	ErrInvalidInvite      = errors.New("invalid invite")
	ErrDuplicateUsername  = errors.New("duplicate username")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrDisabledUser       = errors.New("disabled user")
	ErrInvalidInput       = errors.New("invalid input")
	ErrAdminConflict      = errors.New("admin username conflict")
	ErrPasswordTooShort   = errors.New("password too short")
	ErrUserNotFound       = errors.New("user not found")
)

const MinPasswordLength = 6
```

Add these helpers near the bottom of `service.go`, before `isUniqueViolation`:

```go
func ValidateNewPassword(password string) error {
	if len(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	return nil
}

func hashPassword(password string) (string, error) {
	if err := ValidateNewPassword(password); err != nil {
		return "", err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(passwordHash), nil
}
```

Change `Register` and `EnsureAdmin` so they call `hashPassword(password)` instead of calling `bcrypt.GenerateFromPassword` directly:

```go
	passwordHash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
```

For `EnsureAdmin`, return `err` from `hashPassword` directly:

```go
	passwordHash, err := hashPassword(password)
	if err != nil {
		return err
	}
```

Change SQL parameter usage in those methods from `string(passwordHash)` to `passwordHash`.

Add these service methods after `Login`:

```go
func (s Service) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	if err := ValidateNewPassword(newPassword); err != nil {
		return err
	}

	var passwordHash string
	if err := s.DB.QueryRow(ctx, `
		SELECT password_hash
		FROM users
		WHERE id = $1::uuid
	`, userID).Scan(&passwordHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(currentPassword)); err != nil {
		return ErrInvalidCredentials
	}
	return s.ResetPassword(ctx, userID, newPassword)
}

func (s Service) ResetPassword(ctx context.Context, userID, newPassword string) error {
	passwordHash, err := hashPassword(newPassword)
	if err != nil {
		return err
	}

	tag, err := s.DB.Exec(ctx, `
		UPDATE users
		SET password_hash = $2,
			updated_at = now()
		WHERE id = $1::uuid
	`, userID, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}
```

- [x] **Step 4: Run auth tests to verify they pass**

Run:

```bash
cd api && go test ./internal/auth
```

Expected: PASS.

- [x] **Step 5: Commit**

Run:

```bash
git add api/internal/auth/service.go api/internal/auth/service_test.go
git commit -m "feat: add password update service"
```

---

### Task 2: Admin Password Endpoints

**Files:**
- Modify: `api/internal/admin/handlers.go`
- Modify: `api/internal/admin/handlers_test.go`
- Modify: `api/internal/app/routes.go`

- [x] **Step 1: Write failing handler tests**

In `api/internal/admin/handlers_test.go`, add `bcrypt` to the imports:

```go
"golang.org/x/crypto/bcrypt"
```

Update `setupAdminHandlerTest` route registration:

```go
		r.Post("/password", handlers.ChangeOwnPassword)
		r.Post("/users/{id}/password", handlers.ResetUserPassword)
```

Add these tests before `TestAdminCanDisableUser`:

```go
func TestAdminCanChangeOwnPassword(t *testing.T) {
	ctx, db, handler := setupAdminHandlerTest(t)
	adminID := insertAdminTestUserWithPassword(t, ctx, db, "own-password-admin", models.RoleAdmin, 0, "old-password")

	req := authenticatedAdminJSONRequest(t, http.MethodPost, "/api/admin/password", `{"current_password":"old-password","new_password":"new-password"}`, adminID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var passwordHash string
	if err := db.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, adminID).Scan(&passwordHash); err != nil {
		t.Fatalf("query password hash: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("new-password")); err != nil {
		t.Fatalf("password hash does not verify with new password: %v", err)
	}
	var auditRows int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM audit_logs
		WHERE actor_user_id = $1 AND target_user_id = $1 AND action = 'change_own_password'
			AND metadata::text NOT LIKE '%new-password%'
	`, adminID).Scan(&auditRows); err != nil {
		t.Fatalf("count audit rows: %v", err)
	}
	if auditRows != 1 {
		t.Fatalf("change_own_password audit rows = %d, want 1", auditRows)
	}
}

func TestAdminChangeOwnPasswordRejectsWrongCurrentPassword(t *testing.T) {
	ctx, db, handler := setupAdminHandlerTest(t)
	adminID := insertAdminTestUserWithPassword(t, ctx, db, "bad-current-admin", models.RoleAdmin, 0, "old-password")

	req := authenticatedAdminJSONRequest(t, http.MethodPost, "/api/admin/password", `{"current_password":"wrong-password","new_password":"new-password"}`, adminID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	var passwordHash string
	if err := db.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, adminID).Scan(&passwordHash); err != nil {
		t.Fatalf("query password hash: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("old-password")); err != nil {
		t.Fatalf("old password should still verify: %v", err)
	}
}

func TestAdminCanResetUserPassword(t *testing.T) {
	ctx, db, handler := setupAdminHandlerTest(t)
	adminID := insertAdminTestUserWithPassword(t, ctx, db, "reset-admin", models.RoleAdmin, 0, "admin-password")
	userID := insertAdminTestUserWithPassword(t, ctx, db, "reset-target", models.RoleUser, 0, "old-password")

	req := authenticatedAdminJSONRequest(t, http.MethodPost, "/api/admin/users/"+userID+"/password", `{"new_password":"new-password"}`, adminID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var passwordHash string
	if err := db.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&passwordHash); err != nil {
		t.Fatalf("query password hash: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("new-password")); err != nil {
		t.Fatalf("password hash does not verify with new password: %v", err)
	}
	var auditRows int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM audit_logs
		WHERE actor_user_id = $1 AND target_user_id = $2 AND action = 'reset_user_password'
			AND metadata::text NOT LIKE '%new-password%'
	`, adminID, userID).Scan(&auditRows); err != nil {
		t.Fatalf("count audit rows: %v", err)
	}
	if auditRows != 1 {
		t.Fatalf("reset_user_password audit rows = %d, want 1", auditRows)
	}
}

func TestAdminResetUserPasswordRejectsShortPassword(t *testing.T) {
	ctx, db, handler := setupAdminHandlerTest(t)
	adminID := insertAdminTestUserWithPassword(t, ctx, db, "short-reset-admin", models.RoleAdmin, 0, "admin-password")
	userID := insertAdminTestUserWithPassword(t, ctx, db, "short-reset-user", models.RoleUser, 0, "old-password")

	req := authenticatedAdminJSONRequest(t, http.MethodPost, "/api/admin/users/"+userID+"/password", `{"new_password":"12345"}`, adminID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
```

Add this helper below `insertAdminTestUser`:

```go
func insertAdminTestUserWithPassword(t *testing.T, ctx context.Context, db *pgxpool.Pool, username, role string, credits int, password string) string {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	var userID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, role, status, credit_balance)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text
	`, username, string(hash), role, models.UserStatusActive, credits).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return userID
}
```

- [x] **Step 2: Run tests to verify they fail**

Run:

```bash
cd api && go test ./internal/admin
```

Expected: FAIL with undefined handler methods `ChangeOwnPassword` and `ResetUserPassword`.

- [x] **Step 3: Implement handlers**

In `api/internal/admin/handlers.go`, add these methods after `AdjustCredits`:

```go
func (h Handlers) ChangeOwnPassword(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.CurrentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "请先登录")
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	authService := auth.Service{DB: h.DB}
	if err := authService.ChangePassword(r.Context(), actor.ID, req.CurrentPassword, req.NewPassword); err != nil {
		writePasswordError(w, err)
		return
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	defer tx.Rollback(r.Context())

	if err := insertAuditLog(r.Context(), tx, actor.ID, actor.ID, "change_own_password", map[string]any{"username": actor.Username}); err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h Handlers) ResetUserPassword(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.CurrentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "请先登录")
		return
	}

	userID, ok := validRouteUUID(w, r, "id")
	if !ok {
		return
	}

	var req struct {
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	authService := auth.Service{DB: h.DB}
	if err := authService.ResetPassword(r.Context(), userID, req.NewPassword); err != nil {
		writePasswordError(w, err)
		return
	}

	target, err := findUserSummary(r.Context(), h.DB, userID)
	if err != nil {
		writePasswordError(w, err)
		return
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	defer tx.Rollback(r.Context())

	if err := insertAuditLog(r.Context(), tx, actor.ID, userID, "reset_user_password", map[string]any{"username": target.Username}); err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
```

Add this small type and helper near the response structs:

```go
type userSummary struct {
	ID       string
	Username string
}

func findUserSummary(ctx context.Context, db *pgxpool.Pool, userID string) (userSummary, error) {
	var user userSummary
	err := db.QueryRow(ctx, `
		SELECT id::text, username
		FROM users
		WHERE id = $1::uuid
	`, userID).Scan(&user.ID, &user.Username)
	if errors.Is(err, pgx.ErrNoRows) {
		return userSummary{}, auth.ErrUserNotFound
	}
	if err != nil {
		return userSummary{}, err
	}
	return user, nil
}
```

Add this error writer near `writeCreditError`:

```go
func writePasswordError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrPasswordTooShort):
		writeError(w, http.StatusBadRequest, "新密码至少 6 位")
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "当前密码错误")
	case errors.Is(err, auth.ErrUserNotFound):
		writeError(w, http.StatusNotFound, "用户不存在")
	default:
		writeError(w, http.StatusInternalServerError, "服务器错误")
	}
}
```

In `api/internal/app/routes.go`, register:

```go
		r.Post("/password", a.adminHandlers.ChangeOwnPassword)
		r.Post("/users/{id}/password", a.adminHandlers.ResetUserPassword)
```

- [x] **Step 4: Run admin and app package tests**

Run:

```bash
cd api && go test ./internal/admin ./internal/app
```

Expected: PASS.

- [x] **Step 5: Commit**

Run:

```bash
git add api/internal/admin/handlers.go api/internal/admin/handlers_test.go api/internal/app/routes.go
git commit -m "feat: add admin password endpoints"
```

---

### Task 3: Admin UI Password Controls

**Files:**
- Modify: `web/src/pages/AdminPage.tsx`
- Modify: `web/src/pages/AdminPage.test.tsx`

- [x] **Step 1: Write failing frontend tests**

Update `mockAdminFetch` in `web/src/pages/AdminPage.test.tsx` with these branches:

```ts
    if (path === "/api/admin/password" && init?.method === "POST") {
      return jsonResponse({ ok: true });
    }
    if (path === "/api/admin/users/user-1/password" && init?.method === "POST") {
      return jsonResponse({ ok: true });
    }
```

Add these tests before `does not render image links in audit task table`:

```ts
  test("changes the current admin password from the security tab", async () => {
    const fetchMock = mockAdminFetch();

    render(<AdminPage user={adminUser} />);

    await userEvent.click(await screen.findByRole("tab", { name: "安全" }));
    await userEvent.type(screen.getByLabelText("当前密码"), "old-password");
    await userEvent.type(screen.getByLabelText("新密码"), "new-password");
    await userEvent.type(screen.getByLabelText("确认新密码"), "new-password");
    await userEvent.click(screen.getByRole("button", { name: "更新密码" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/admin/password",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ current_password: "old-password", new_password: "new-password" }),
        }),
      );
    });
    expect(await screen.findByText("密码已更新")).toBeInTheDocument();
  });

  test("resets a user password from the users table", async () => {
    const fetchMock = mockAdminFetch();

    render(<AdminPage user={adminUser} />);

    const row = await screen.findByRole("row", { name: /alice/ });
    await userEvent.click(within(row).getByRole("button", { name: "重置密码" }));
    await userEvent.type(screen.getByLabelText("alice 的新密码"), "new-password");
    await userEvent.click(screen.getByRole("button", { name: "确认重置" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/admin/users/user-1/password",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ new_password: "new-password" }),
        }),
      );
    });
    expect(await screen.findByText("用户密码已重置")).toBeInTheDocument();
  });

  test("shows the simple credit rule in the credit tab", async () => {
    mockAdminFetch();

    render(<AdminPage user={adminUser} />);

    await userEvent.click(await screen.findByRole("tab", { name: "额度" }));

    expect(screen.getByText("点数规则：每次生成扣 1 点，失败自动退回 1 点。")).toBeInTheDocument();
  });
```

- [x] **Step 2: Run tests to verify they fail**

Run:

```bash
cd web && npm test -- AdminPage.test.tsx --run
```

Expected: FAIL because the “安全” tab and reset-password controls do not exist.

- [x] **Step 3: Implement AdminPage state and handlers**

In `web/src/pages/AdminPage.tsx`, change the `AdminTab` type and `tabs` array:

```ts
type AdminTab = "users" | "invites" | "credits" | "security" | "audit";

const tabs: Array<{ id: AdminTab; label: string }> = [
  { id: "users", label: "用户" },
  { id: "invites", label: "邀请码" },
  { id: "credits", label: "额度" },
  { id: "security", label: "安全" },
  { id: "audit", label: "审计" },
];
```

Add state after `creditDrafts`:

```ts
  const [ownPasswordDraft, setOwnPasswordDraft] = useState({
    currentPassword: "",
    newPassword: "",
    confirmPassword: "",
  });
  const [resetPasswordUserId, setResetPasswordUserId] = useState("");
  const [resetPasswordDraft, setResetPasswordDraft] = useState("");
  const [notice, setNotice] = useState("");
```

Clear `notice` at the start of submit handlers by adding `setNotice("");` next to `setError("");`.

Add these handlers after `handleCreditSubmit`:

```ts
  async function handleOwnPasswordSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy("own-password");
    setError("");
    setNotice("");

    if (ownPasswordDraft.newPassword !== ownPasswordDraft.confirmPassword) {
      setError("两次输入的新密码不一致");
      setBusy("");
      return;
    }

    try {
      await api<{ ok: boolean }>("/api/admin/password", {
        method: "POST",
        body: JSON.stringify({
          current_password: ownPasswordDraft.currentPassword,
          new_password: ownPasswordDraft.newPassword,
        }),
      });
      setOwnPasswordDraft({ currentPassword: "", newPassword: "", confirmPassword: "" });
      setNotice("密码已更新");
    } catch (err) {
      setError(err instanceof Error ? err.message : "更新密码失败");
    } finally {
      setBusy("");
    }
  }

  async function handleResetPasswordSubmit(event: FormEvent<HTMLFormElement>, target: AdminUser) {
    event.preventDefault();
    setBusy(`reset-password-${target.id}`);
    setError("");
    setNotice("");

    try {
      await api<{ ok: boolean }>(`/api/admin/users/${target.id}/password`, {
        method: "POST",
        body: JSON.stringify({ new_password: resetPasswordDraft }),
      });
      setResetPasswordUserId("");
      setResetPasswordDraft("");
      setNotice("用户密码已重置");
    } catch (err) {
      setError(err instanceof Error ? err.message : "重置密码失败");
    } finally {
      setBusy("");
    }
  }
```

Render the notice below the error:

```tsx
      {notice ? <p className="form-success" role="status">{notice}</p> : null}
```

In the users table, change the operations cell so it includes both buttons:

```tsx
                      <button
                        className="secondary-button compact-button"
                        disabled={busy === `status-${item.id}`}
                        onClick={() => void handleStatusChange(item, item.status === "active" ? "disabled" : "active")}
                        type="button"
                      >
                        {item.status === "active" ? "禁用" : "启用"}
                      </button>
                      <button
                        className="secondary-button compact-button"
                        onClick={() => {
                          setResetPasswordUserId(item.id);
                          setResetPasswordDraft("");
                          setNotice("");
                          setError("");
                        }}
                        type="button"
                      >
                        重置密码
                      </button>
```

After each user row, render the inline reset row:

```tsx
                  {resetPasswordUserId === item.id ? (
                    <tr>
                      <td colSpan={6}>
                        <form className="inline-admin-form" onSubmit={(event) => void handleResetPasswordSubmit(event, item)}>
                          <label className="field inline-field">
                            <span>{item.username} 的新密码</span>
                            <input
                              aria-label={`${item.username} 的新密码`}
                              minLength={6}
                              onChange={(event) => setResetPasswordDraft(event.target.value)}
                              type="password"
                              value={resetPasswordDraft}
                            />
                          </label>
                          <button className="primary-button compact-button" disabled={busy === `reset-password-${item.id}`} type="submit">
                            确认重置
                          </button>
                          <button className="secondary-button compact-button" onClick={() => setResetPasswordUserId("")} type="button">
                            取消
                          </button>
                        </form>
                      </td>
                    </tr>
                  ) : null}
```

In the credits tab section, add this before `table-wrap`:

```tsx
          <p className="muted-text">点数规则：每次生成扣 1 点，失败自动退回 1 点。</p>
```

Add the security tab section before the audit section:

```tsx
      {!loading && activeTab === "security" ? (
        <section className="admin-section panel" aria-labelledby="security-title">
          <h3 id="security-title">账号安全</h3>
          <form className="compact-form" onSubmit={handleOwnPasswordSubmit}>
            <label className="field">
              <span>当前密码</span>
              <input
                autoComplete="current-password"
                onChange={(event) => setOwnPasswordDraft((current) => ({ ...current, currentPassword: event.target.value }))}
                required
                type="password"
                value={ownPasswordDraft.currentPassword}
              />
            </label>
            <label className="field">
              <span>新密码</span>
              <input
                autoComplete="new-password"
                minLength={6}
                onChange={(event) => setOwnPasswordDraft((current) => ({ ...current, newPassword: event.target.value }))}
                required
                type="password"
                value={ownPasswordDraft.newPassword}
              />
            </label>
            <label className="field">
              <span>确认新密码</span>
              <input
                autoComplete="new-password"
                minLength={6}
                onChange={(event) => setOwnPasswordDraft((current) => ({ ...current, confirmPassword: event.target.value }))}
                required
                type="password"
                value={ownPasswordDraft.confirmPassword}
              />
            </label>
            <button className="primary-button" disabled={busy === "own-password"} type="submit">
              更新密码
            </button>
          </form>
        </section>
      ) : null}
```

- [x] **Step 4: Run AdminPage tests**

Run:

```bash
cd web && npm test -- AdminPage.test.tsx --run
```

Expected: PASS.

- [x] **Step 5: Commit**

Run:

```bash
git add web/src/pages/AdminPage.tsx web/src/pages/AdminPage.test.tsx
git commit -m "feat: add admin password controls"
```

---

### Task 4: User-Facing Rule Copy

**Files:**
- Modify: `web/src/pages/WorkspacePage.tsx`
- Modify: `web/src/pages/WorkspacePage.test.tsx`
- Modify: `web/src/pages/HistoryPage.tsx`
- Modify: `web/src/pages/HistoryPage.test.tsx`
- Modify: `README.md`

- [x] **Step 1: Write failing UI copy tests**

In `web/src/pages/WorkspacePage.test.tsx`, add this test before `creates a generation with prompt and ratio`:

```ts
  test("shows simple point, refund, and retention guidance", () => {
    render(<WorkspacePage user={user} />);

    expect(screen.getByText("输入提示词，选择画面比例后开始生成。每次生成 1 张图，扣 1 点；失败会自动退回点数。生成图片保留 30 天。")).toBeInTheDocument();
  });
```

Replace the existing failure-message test with:

```ts
  test("shows fixed refunded failure message", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      jsonResponse({
        task: {
          id: "task-3",
          prompt: "海边",
          ratio: "1:1",
          size: "1024x1024",
          status: "failed",
          error_message: "upstream internal details",
          created_at: "2026-04-30T08:00:00Z",
          completed_at: "2026-04-30T08:01:00Z",
        },
      }),
    );

    render(<WorkspacePage user={user} />);

    await userEvent.type(screen.getByLabelText("提示词"), "海边");
    await userEvent.click(screen.getByRole("button", { name: "生成" }));

    expect(await screen.findByText("生成失败，已退回 1 点，可调整提示词后重试。")).toBeInTheDocument();
    expect(screen.queryByText("upstream internal details")).not.toBeInTheDocument();
  });
```

In `web/src/pages/HistoryPage.test.tsx`, add this assertion after `render(<HistoryPage />);`:

```ts
    expect(screen.getByText("这里只显示最近 30 天的生成记录。请及时下载需要长期保存的图片。")).toBeInTheDocument();
```

- [x] **Step 2: Run tests to verify they fail**

Run:

```bash
cd web && npm test -- WorkspacePage.test.tsx HistoryPage.test.tsx --run
```

Expected: FAIL because the new guidance copy is not rendered and failure details are still shown.

- [x] **Step 3: Implement copy and failure display**

In `web/src/pages/WorkspacePage.tsx`, add this paragraph after the balance row:

```tsx
          <p className="usage-note">
            输入提示词，选择画面比例后开始生成。每次生成 1 张图，扣 1 点；失败会自动退回点数。生成图片保留 30 天。
          </p>
```

Change failed task rendering to:

```tsx
              {currentTask.status === "failed" ? (
                <p className="form-error" role="alert">
                  生成失败，已退回 1 点，可调整提示词后重试。
                </p>
              ) : null}
```

In `web/src/pages/HistoryPage.tsx`, add this paragraph in the section heading under the `h2`:

```tsx
          <p className="muted-text">这里只显示最近 30 天的生成记录。请及时下载需要长期保存的图片。</p>
```

In `README.md`, add this section after the opening description:

```markdown
## 使用规则

- 每次生成 1 张图片，创建任务时扣 1 点。
- 生成失败会自动退回 1 点，系统不自动重试；用户可调整提示词后重新提交。
- 生成图片和历史记录面向用户展示最近 30 天，请及时下载需要长期保存的图片。
```

- [x] **Step 4: Run UI copy tests**

Run:

```bash
cd web && npm test -- WorkspacePage.test.tsx HistoryPage.test.tsx --run
```

Expected: PASS.

- [x] **Step 5: Commit**

Run:

```bash
git add web/src/pages/WorkspacePage.tsx web/src/pages/WorkspacePage.test.tsx web/src/pages/HistoryPage.tsx web/src/pages/HistoryPage.test.tsx README.md
git commit -m "feat: add real use guidance copy"
```

---

### Task 5: Full Verification

**Files:**
- Verify all modified files.

- [x] **Step 1: Run full Go test suite**

Run:

```bash
cd api && go test ./...
```

Expected: PASS.

- [x] **Step 2: Run full web test suite**

Run:

```bash
cd web && npm test -- --run
```

Expected: PASS.

- [x] **Step 3: Build frontend**

Run:

```bash
cd web && npm run build
```

Expected: PASS with Vite production build output.

- [x] **Step 4: Validate Docker Compose config**

Run:

```bash
docker compose config
```

Expected: PASS and prints the resolved Compose configuration.

- [x] **Step 5: Inspect final diff**

Run:

```bash
git status --short
git log --oneline -5
```

Expected: working tree clean after the task commits, with recent commits for auth service, admin endpoints, admin UI controls, and user-facing guidance.

---

## Self-Review Notes

- Spec coverage: admin self password change is Task 1 and Task 2, admin reset password is Task 1 through Task 3, simple point rule is Task 4 plus AdminPage copy in Task 3, failed refund/no retry copy is Task 4, 30-day retention prompt is Task 4, README rule documentation is Task 4.
- Placeholder scan: this plan contains concrete files, code snippets, commands, and expected outcomes for every task.
- Type consistency: backend JSON fields use `current_password` and `new_password`; frontend requests match those fields. Frontend tab id is `security`, label is `安全`, and tests query the visible label.
