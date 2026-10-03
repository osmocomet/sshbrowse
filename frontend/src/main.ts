import { mount } from 'svelte'
import App from './App.svelte'
import './app.css'
import { loadPreferences } from './lib/storage'

try {
  const appearance = loadPreferences(localStorage)
  document.documentElement.dataset.theme = appearance.themeName
  document.documentElement.dataset.terminalColors = appearance.terminalColors
  document.documentElement.dataset.uiSize = appearance.uiSize
} catch {
  // The default palette remains usable when WebView storage is unavailable.
}

mount(App, { target: document.getElementById('app')! })
