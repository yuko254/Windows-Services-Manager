import React, { useState, createContext, useContext } from 'react'
import {createRoot} from 'react-dom/client'
import './style.css'
import App from './App'
import { FluentProvider, webDarkTheme, webLightTheme } from '@fluentui/react-components'

export const ThemeContext = createContext();

const ThemeProvider = ({ children }) => {
  const [isDark, setIsDark] = useState(false);
  const toggleTheme = () => setIsDark(prev => !prev);
  const currentTheme = isDark ? webDarkTheme : webLightTheme;

  return (
    <ThemeContext.Provider value={{ isDark, toggleTheme }}>
      <FluentProvider theme={currentTheme}>
        {children}
      </FluentProvider>
    </ThemeContext.Provider>
  );
};

const container = document.getElementById('root')
const root = createRoot(container)

root.render(
  <React.StrictMode>
    <ThemeProvider>
      <App />
    </ThemeProvider>
  </React.StrictMode>
);
