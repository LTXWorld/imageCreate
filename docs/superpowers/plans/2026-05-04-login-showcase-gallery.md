# Login Showcase Gallery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a six-image pure visual showcase to the unauthenticated login/home page.

**Architecture:** The login page will own a small fixed showcase image array and render it through a focused `ShowcaseGallery` helper component in `LoginPage.tsx`. Static image files will be copied into `web/public/showcase/` so Vite can serve them by stable `/showcase/<file>.png` URLs without bundler imports.

**Tech Stack:** React, TypeScript, Vite public assets, CSS, Vitest, Testing Library.

---

## File Structure

- Modify `web/src/pages/LoginPage.tsx`: add the showcase image list, render a two-column login/showcase composition, and keep the existing login form behavior intact.
- Modify `web/src/styles/app.css`: add responsive layout and image grid styles that fit the existing app palette.
- Modify `web/src/App.test.tsx`: assert unauthenticated users see the six showcase images and restored authenticated sessions do not show the login showcase.
- Create `web/public/showcase/罗威纳.png`, `web/public/showcase/伯恩山.png`, `web/public/showcase/恭王府.png`, `web/public/showcase/陈平安.png`, `web/public/showcase/左右.png`, `web/public/showcase/起床.png`: browser-readable static showcase assets copied from `pic/`.

---

### Task 1: Test Login Showcase Visibility

**Files:**
- Modify: `web/src/App.test.tsx`

- [ ] **Step 1: Add constants near the imports**

Add this constant after the `import { App } from "./App";` line:

```ts
const showcaseAltTexts = [
  "罗威纳展示图",
  "伯恩山展示图",
  "恭王府展示图",
  "陈平安展示图",
  "左右展示图",
  "起床展示图",
];
```

- [ ] **Step 2: Add a failing unauthenticated login showcase test**

Add this test as the first test inside `describe("App", () => {`:

```ts
test("shows showcase images on the unauthenticated login page", async () => {
  vi.spyOn(globalThis, "fetch").mockRejectedValue(new Error("not authenticated"));

  render(<App />);

  await waitFor(() => {
    expect(screen.getByRole("heading", { name: "登录" })).toBeInTheDocument();
  });

  const gallery = screen.getByLabelText("生成效果展示");
  for (const altText of showcaseAltTexts) {
    expect(within(gallery).getByAltText(altText)).toBeInTheDocument();
  }
});
```

- [ ] **Step 3: Add a failing authenticated-session absence assertion**

In the existing `restores an existing session from auth me on startup` test, after this line:

```ts
expect(screen.getByText("图像生成")).toBeInTheDocument();
```

add:

```ts
expect(screen.queryByLabelText("生成效果展示")).not.toBeInTheDocument();
```

- [ ] **Step 4: Run the focused test to verify failure**

Run:

```bash
cd web && npm test -- --run src/App.test.tsx
```

Expected: FAIL because the login page does not yet render an element labeled `生成效果展示`.

---

### Task 2: Add Static Showcase Assets

**Files:**
- Create: `web/public/showcase/罗威纳.png`
- Create: `web/public/showcase/伯恩山.png`
- Create: `web/public/showcase/恭王府.png`
- Create: `web/public/showcase/陈平安.png`
- Create: `web/public/showcase/左右.png`
- Create: `web/public/showcase/起床.png`

- [ ] **Step 1: Create the public asset directory**

Run:

```bash
mkdir -p web/public/showcase
```

Expected: command exits successfully.

- [ ] **Step 2: Copy the six provided images**

Run:

```bash
cp pic/罗威纳.png web/public/showcase/罗威纳.png
cp pic/伯恩山.png web/public/showcase/伯恩山.png
cp pic/恭王府.png web/public/showcase/恭王府.png
cp pic/陈平安.png web/public/showcase/陈平安.png
cp pic/左右.png web/public/showcase/左右.png
cp pic/起床.png web/public/showcase/起床.png
```

Expected: all six files exist in `web/public/showcase/`.

- [ ] **Step 3: Verify copied dimensions**

Run:

```bash
sips -g pixelWidth -g pixelHeight web/public/showcase/*.png
```

Expected: five images report `1024 x 1024`, and `起床.png` reports `1122 x 1402`.

---

### Task 3: Render Showcase Gallery On Login Page

**Files:**
- Modify: `web/src/pages/LoginPage.tsx`

- [ ] **Step 1: Add the showcase image data**

Add this after the `LoginPageProps` type:

```ts
const showcaseImages = [
  { alt: "罗威纳展示图", src: "/showcase/罗威纳.png" },
  { alt: "伯恩山展示图", src: "/showcase/伯恩山.png" },
  { alt: "恭王府展示图", src: "/showcase/恭王府.png" },
  { alt: "陈平安展示图", src: "/showcase/陈平安.png" },
  { alt: "左右展示图", src: "/showcase/左右.png" },
  { alt: "起床展示图", src: "/showcase/起床.png" },
];
```

- [ ] **Step 2: Add the helper component**

Add this below the `showcaseImages` constant:

```tsx
function ShowcaseGallery() {
  return (
    <section className="login-showcase" aria-label="生成效果展示">
      <div className="showcase-grid">
        {showcaseImages.map((image) => (
          <img className="showcase-image" key={image.src} src={image.src} alt={image.alt} />
        ))}
      </div>
    </section>
  );
}
```

- [ ] **Step 3: Wrap the existing login card and render the gallery beside it**

Replace the current top-level `return (` body in `LoginPage` with:

```tsx
  return (
    <section className="login-home" aria-label="账号登录">
      <section className="auth-surface login-card" aria-labelledby="login-title">
        <div className="section-heading">
          <p className="eyebrow">已有账号</p>
          <h2 id="login-title">登录</h2>
        </div>

        <form className="auth-form" onSubmit={handleSubmit}>
          <label className="field">
            <span>用户名</span>
            <input
              autoComplete="username"
              name="username"
              onChange={(event) => setUsername(event.target.value)}
              required
              value={username}
            />
          </label>

          <label className="field">
            <span>密码</span>
            <input
              autoComplete="current-password"
              name="password"
              onChange={(event) => setPassword(event.target.value)}
              required
              type="password"
              value={password}
            />
          </label>

          {error ? <p className="form-error" role="alert">{error}</p> : null}

          <div className="form-actions">
            <button className="primary-button" disabled={loading} type="submit">
              {loading ? "登录中..." : "登录"}
            </button>
            <button className="secondary-button" type="button" onClick={onRegisterClick}>
              去注册
            </button>
          </div>
        </form>
      </section>

      <ShowcaseGallery />
    </section>
  );
```

- [ ] **Step 4: Run the focused test**

Run:

```bash
cd web && npm test -- --run src/App.test.tsx
```

Expected: test still may fail visually or by asset-independent assertions until CSS is added, but the showcase accessibility assertions should now pass.

---

### Task 4: Style The Login Showcase Layout

**Files:**
- Modify: `web/src/styles/app.css`

- [ ] **Step 1: Add desktop login/showcase styles**

Add this after the `.auth-surface { ... }` block:

```css
.login-home {
  display: grid;
  grid-template-columns: minmax(320px, 440px) minmax(0, 1fr);
  gap: 24px;
  align-items: center;
  width: 100%;
}

.login-card {
  width: 100%;
}

.login-showcase {
  min-width: 0;
}

.showcase-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}

.showcase-image {
  display: block;
  width: 100%;
  aspect-ratio: 1 / 1;
  object-fit: cover;
  border: 1px solid var(--color-jade-light);
  border-radius: 12px;
  background: #fdfdfd;
  box-shadow: var(--shadow-ethereal);
}
```

- [ ] **Step 2: Add mobile responsive behavior**

Inside the existing `@media (max-width: 760px) { ... }` block, add `.login-home` to the selector that already includes `.section-toolbar, .workspace-grid, .admin-grid, .history-item` so it becomes:

```css
  .section-toolbar,
  .login-home,
  .workspace-grid,
  .admin-grid,
  .history-item {
    grid-template-columns: 1fr;
  }
```

Then add this before the closing brace of the media query:

```css
  .login-home {
    align-items: stretch;
  }

  .showcase-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
```

- [ ] **Step 3: Run the focused test**

Run:

```bash
cd web && npm test -- --run src/App.test.tsx
```

Expected: PASS.

---

### Task 5: Verify And Commit

**Files:**
- Verify all changed frontend files and copied assets.

- [ ] **Step 1: Run the full frontend test suite**

Run:

```bash
cd web && npm test -- --run
```

Expected: PASS.

- [ ] **Step 2: Build the frontend**

Run:

```bash
cd web && npm run build
```

Expected: PASS and Vite emits the production build.

- [ ] **Step 3: Inspect git status**

Run:

```bash
git status --short
```

Expected: changed files include `web/src/pages/LoginPage.tsx`, `web/src/styles/app.css`, `web/src/App.test.tsx`, and the six `web/public/showcase/*.png` files. The original `pic/` directory can remain untracked unless the team wants to keep source assets in git.

- [ ] **Step 4: Commit implementation**

Run:

```bash
git add web/src/pages/LoginPage.tsx web/src/styles/app.css web/src/App.test.tsx web/public/showcase
git commit -m "feat: add login showcase gallery"
```

Expected: commit succeeds.

---

## Self-Review

- Spec coverage: the plan renders exactly six images, keeps them pure image-only, uses the selected side-by-side login layout, adds mobile stacking, includes alt text and section labeling, and preserves authenticated routing.
- Placeholder scan: no placeholders or deferred implementation notes remain.
- Type consistency: `showcaseImages`, `ShowcaseGallery`, `login-home`, `login-showcase`, `showcase-grid`, and `showcase-image` are named consistently across code, CSS, and tests.
