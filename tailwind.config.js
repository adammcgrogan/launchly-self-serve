// Source config for precompiling web/static/css/app.css. Not part of the
// runtime app (Go has no npm/node dependency) — this is a dev-only input for
// regenerating the stylesheet when template class names change:
//
//   npx tailwindcss@3 -i web/static/css/input.css -o web/static/css/app.css --minify
//
// Theme values mirror what the old cdn.tailwindcss.com per-page configs set.
module.exports = {
  content: [
    "./web/templates/**/*.html",
    "./internal/**/*.go",
  ],
  theme: {
    extend: {
      fontFamily: {
        sans: ["Inter", "-apple-system", "sans-serif"],
        display: ["Fraunces", "serif"],
      },
      colors: {
        brand: { DEFAULT: "#4F46E5", dark: "#3730A3" },
        amber: { DEFAULT: "#F59E0B" },
      },
    },
  },
  plugins: [],
};
