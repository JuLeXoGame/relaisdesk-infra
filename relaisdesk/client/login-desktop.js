(function() {
  const params = new URLSearchParams(window.location.search);
  const port = params.get('port');
  const state = params.get('state');

  const messageBox = document.getElementById('authMessage');
  const googleSection = document.getElementById('googleSection');
  const redirectHelp = document.getElementById('redirectHelp');
  const fallbackLink = document.getElementById('fallbackRedirectLink');

  function showMessage(text, isError) {
    if (!messageBox) return;
    messageBox.textContent = text;
    messageBox.style.display = 'block';
    if (isError) {
      messageBox.className = 'banner error';
      messageBox.style.background = 'rgba(239, 68, 68, 0.15)';
      messageBox.style.borderColor = 'rgba(239, 68, 68, 0.4)';
      messageBox.style.color = '#fca5a5';
    } else {
      messageBox.className = 'banner success';
      messageBox.style.background = 'rgba(34, 197, 94, 0.15)';
      messageBox.style.borderColor = 'rgba(34, 197, 94, 0.4)';
      messageBox.style.color = '#86efac';
    }
  }

  // Validate query parameters
  const portNum = parseInt(port, 10);
  if (!port || isNaN(portNum) || portNum < 1024 || portNum > 65535 || !state || state.length < 16) {
    showMessage('Paramètres de connexion invalides ou expirés. Veuillez relancer la connexion depuis votre application RelaisDesk Technicien.', true);
    if (googleSection) googleSection.style.display = 'none';
    return;
  }

  function handleDesktopGoogleLogin(response) {
    if (!response || !response.credential) {
      showMessage('Impossible de récupérer les informations de connexion Google.', true);
      return;
    }

    showMessage('✓ Connexion Google confirmée. Redirection vers RelaisDesk Technicien en cours…', false);
    if (googleSection) googleSection.style.display = 'none';

    const callbackUrl = 'http://127.0.0.1:' + portNum + '/callback?credential=' + encodeURIComponent(response.credential) + '&state=' + encodeURIComponent(state);

    if (fallbackLink) {
      fallbackLink.href = callbackUrl;
    }
    if (redirectHelp) {
      redirectHelp.style.display = 'block';
    }

    // Redirect immediately to local loopback server
    window.location.href = callbackUrl;
  }

  function initGoogleSignIn() {
    const container = document.getElementById('googleSignInBtn');
    if (!container) return;

    if (typeof google === 'undefined' || !google.accounts || !google.accounts.id) {
      setTimeout(initGoogleSignIn, 150);
      return;
    }

    try {
      google.accounts.id.initialize({
        client_id: '226991768980-c2rcfkicoft0hl9m346n45ad088utri9.apps.googleusercontent.com',
        callback: handleDesktopGoogleLogin,
        auto_select: false
      });

      google.accounts.id.renderButton(container, {
        type: 'standard',
        theme: 'filled_blue',
        size: 'large',
        text: 'signin_with',
        shape: 'rectangular',
        logo_alignment: 'left',
        width: 300
      });
    } catch (err) {
      console.error('Erreur initialisation Google Sign-In:', err);
      showMessage('Erreur lors du chargement du module Google Sign-In.', true);
    }
  }

  // Start initialization
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initGoogleSignIn);
  } else {
    initGoogleSignIn();
  }
})();
