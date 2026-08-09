import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'

export default [
  { ignores: ['dist'] },
  { ...js.configs.recommended, files: ['**/*.{js,jsx}'] },
  { ...reactHooks.configs.flat.recommended, files: ['**/*.{js,jsx}'] },
  { ...reactRefresh.configs.vite, files: ['**/*.{js,jsx}'] },
  {
    files: ['**/*.{js,jsx}'],
    languageOptions: {
      ecmaVersion: 'latest',
      globals: globals.browser,
      parserOptions: {
        ecmaFeatures: { jsx: true },
        sourceType: 'module',
      },
    },
    rules: {
      'no-unused-vars': ['error', { argsIgnorePattern: '^(?:_|[A-Z])', varsIgnorePattern: '^[A-Z_]' }],
      'react-hooks/set-state-in-effect': 'off',
    },
  },
  {
    files: ['src/main.jsx'],
    rules: { 'react-refresh/only-export-components': 'off' },
  },
]
