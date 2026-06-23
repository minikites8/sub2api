/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // 主色调 - YouTube Studio neutral gray，红色只由少量品牌/主操作 token 使用
        primary: {
          50: '#f8f8f8',
          100: '#f2f2f2',
          200: '#e5e5e5',
          300: '#d9d9d9',
          400: '#aaaaaa',
          500: '#717171',
          600: '#606060',
          700: '#3f3f3f',
          800: '#272727',
          900: '#181818',
          950: '#0f0f0f'
        },
        // 辅助色 - 深蓝灰
        accent: {
          50: '#f8fafc',
          100: '#f1f5f9',
          200: '#e2e8f0',
          300: '#cbd5e1',
          400: '#94a3b8',
          500: '#64748b',
          600: '#475569',
          700: '#334155',
          800: '#1e293b',
          900: '#0f172a',
          950: '#020617'
        },
        // 深色模式背景 - YouTube Studio surfaces
        dark: {
          50: '#f1f1f1',
          100: '#e5e5e5',
          200: '#cccccc',
          300: '#aaaaaa',
          400: '#909090',
          500: '#717171',
          600: '#606060',
          700: '#3f3f3f',
          800: '#272727',
          900: '#181818',
          950: '#0f0f0f'
        }
      },
      fontFamily: {
        sans: [
          'system-ui',
          '-apple-system',
          'BlinkMacSystemFont',
          'Segoe UI',
          'Roboto',
          'Helvetica Neue',
          'Arial',
          'PingFang SC',
          'Hiragino Sans GB',
          'Microsoft YaHei',
          'sans-serif'
        ],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace']
      },
      boxShadow: {
        glass: '0 1px 2px rgba(0, 0, 0, 0.08)',
        'glass-sm': '0 1px 1px rgba(0, 0, 0, 0.06)',
        glow: '0 1px 2px rgba(0, 0, 0, 0.08)',
        'glow-lg': '0 2px 6px rgba(0, 0, 0, 0.1)',
        card: '0 1px 2px rgba(0, 0, 0, 0.08)',
        'card-hover': '0 2px 6px rgba(0, 0, 0, 0.1)',
        'inner-glow': 'inset 0 0 0 1px rgba(255, 255, 255, 0.04)'
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-primary': 'linear-gradient(0deg, #0f0f0f 0%, #0f0f0f 100%)',
        'gradient-dark': 'linear-gradient(0deg, #0f0f0f 0%, #0f0f0f 100%)',
        'gradient-glass':
          'linear-gradient(0deg, rgba(255,255,255,0.92) 0%, rgba(255,255,255,0.92) 100%)',
        'mesh-gradient':
          'linear-gradient(180deg, rgba(255, 255, 255, 1) 0%, rgba(255, 255, 255, 1) 100%)'
      },
      animation: {
        'fade-in': 'fadeIn 0.3s ease-out',
        'slide-up': 'slideUp 0.3s ease-out',
        'slide-down': 'slideDown 0.3s ease-out',
        'slide-in-right': 'slideInRight 0.3s ease-out',
        'scale-in': 'scaleIn 0.2s ease-out',
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2s linear infinite',
        glow: 'none'
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' }
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideDown: {
          '0%': { opacity: '0', transform: 'translateY(-10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideInRight: {
          '0%': { opacity: '0', transform: 'translateX(20px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' }
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.95)' },
          '100%': { opacity: '1', transform: 'scale(1)' }
        },
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' }
        },
        glow: {
          '0%': { boxShadow: '0 1px 2px rgba(0, 0, 0, 0.08)' },
          '100%': { boxShadow: '0 1px 2px rgba(0, 0, 0, 0.08)' }
        }
      },
      backdropBlur: {
        xs: '2px'
      },
      borderRadius: {
        '4xl': '2rem'
      }
    }
  },
  plugins: []
}
