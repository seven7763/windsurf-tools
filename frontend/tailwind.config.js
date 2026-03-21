/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{vue,js,ts,jsx,tsx}",
  ],
  darkMode: 'class',
  theme: {
    extend: {
      fontFamily: {
        sans: [
          '-apple-system',
          'BlinkMacSystemFont',
          '"SF Pro Text"',
          '"Segoe UI"',
          'Roboto',
          'Helvetica',
          'Arial',
          'sans-serif',
        ],
      },
      colors: {
        ios: {
          bg: '#F2F2F7', 
          bgDark: '#000000',
          card: '#FFFFFF',
          cardDark: '#1C1C1E',
          blue: '#007AFF',
          blueDark: '#0A84FF',
          green: '#34C759',
          greenDark: '#30D158',
          red: '#FF3B30',
          redDark: '#FF453A',
          text: '#000000',
          textDark: '#FFFFFF',
          textSecondary: 'rgba(60, 60, 67, 0.6)',
          textSecondaryDark: 'rgba(235, 235, 245, 0.6)',
          divider: 'rgba(60, 60, 67, 0.29)',
          dividerDark: 'rgba(84, 84, 88, 0.65)'
        }
      }
    },
  },
  plugins: [],
}
