export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
      primary: {
        50: '#f3f0ff',
        100: '#e5dbff',
        200: '#d0bfff',
        300: '#b197fc',
        400: '#9775fa',
        500: '#845ef7',
        600: '#7950f2',
        700: '#6741d9',
        800: '#5a2e8a',
        900: '#3b1f6e',
      },
      dark: {
        700: '#1e3a5f',
        800: '#0f2440',
        900: '#0a1628',
      },
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'sans-serif'],
      },
    },
  },
  plugins: [],
};