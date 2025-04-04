// Check for saved theme preference or use the system preference
function getThemePreference() {
    if (localStorage.getItem('theme') === 'dark' || 
        (!localStorage.getItem('theme') && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
      return 'dark';
    }
    return 'light';
  }
  
  // Function to set the theme
  function setTheme(theme) {
    if (theme === 'dark') {
      document.documentElement.classList.add('dark');
      localStorage.setItem('theme', 'dark');
    } else {
      document.documentElement.classList.remove('dark');
      localStorage.setItem('theme', 'light');
    }
    
    // Update the toggle button icon
    updateToggleIcon();
  }
  
  // Function to update the toggle button icon based on current theme
  function updateToggleIcon() {
    const isDark = document.documentElement.classList.contains('dark');
    const toggleIcons = document.querySelectorAll('.theme-toggle-icon');
    
    toggleIcons.forEach(icon => {
      // Sun icon for dark mode (showing what clicking will change to)
      const sunIcon = icon.querySelector('.sun-icon');
      // Moon icon for light mode (showing what clicking will change to)
      const moonIcon = icon.querySelector('.moon-icon');
      
      if (isDark) {
        moonIcon.classList.add('hidden');
        sunIcon.classList.remove('hidden');
      } else {
        sunIcon.classList.add('hidden');
        moonIcon.classList.remove('hidden');
      }
    });
  }
  
  // Function to toggle the theme
  function toggleTheme() {
    const isDark = document.documentElement.classList.contains('dark');
    setTheme(isDark ? 'light' : 'dark');
  }
  
  // Initialize theme on page load
  document.addEventListener('DOMContentLoaded', () => {
    // Set initial theme
    const theme = getThemePreference();
    setTheme(theme);
    
    // Add event listeners to toggle buttons
    const toggleButtons = document.querySelectorAll('.theme-toggle');
    toggleButtons.forEach(button => {
      button.addEventListener('click', toggleTheme);
    });
  });