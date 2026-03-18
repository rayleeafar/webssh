/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    './pages/**/*.{js,ts,jsx,tsx,mdx}',
    './components/**/*.{js,ts,jsx,tsx,mdx}',
    './app/**/*.{js,ts,jsx,tsx,mdx}',
  ],
  theme: {
    extend: {
      colors: {
        void: '#050508',
        deep: '#080810',
        surface: '#0d0d1a',
        elevated: '#111120',
        border: {
          dim: 'rgba(0,255,255,0.12)',
          mid: 'rgba(0,255,255,0.3)',
          bright: 'rgba(0,255,255,0.7)',
        },
        neon: {
          cyan: '#00ffff',
          magenta: '#ff00ff',
          blue: '#0080ff',
          green: '#00ff88',
          red: '#ff3060',
        },
        ink: {
          primary: '#c8d8f0',
          secondary: '#6070a0',
          dim: '#303060',
        },
      },
      fontFamily: {
        display: ['Orbitron', 'sans-serif'],
        body: ['Rajdhani', 'sans-serif'],
        mono: ['JetBrains Mono', 'Menlo', 'Monaco', 'Courier New', 'monospace'],
      },
      keyframes: {
        blink: {
          '0%, 100%': { opacity: '1' },
          '50%': { opacity: '0' },
        },
        pulse_glow: {
          '0%, 100%': { boxShadow: '0 0 4px rgba(0,255,255,0.4), 0 0 12px rgba(0,255,255,0.1)' },
          '50%': { boxShadow: '0 0 8px rgba(0,255,255,0.8), 0 0 24px rgba(0,255,255,0.3)' },
        },
        scan: {
          '0%': { transform: 'translateY(-100%)' },
          '100%': { transform: 'translateY(100vh)' },
        },
      },
      animation: {
        blink: 'blink 1s step-end infinite',
        pulse_glow: 'pulse_glow 2s ease-in-out infinite',
        scan: 'scan 8s linear infinite',
      },
    },
  },
  plugins: [],
}
