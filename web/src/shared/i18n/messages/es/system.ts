export const system = {
  // Ajustes
  "system.settings.description":
    "Ajustes de flota. Aplican en caliente salvo binarios, que exigen reinicio del servicio.",
  "system.settings.keys.one": "{n} clave",
  "system.settings.keys.other": "{n} claves",
  "system.settings.githubConnected": "GitHub conectado ✓ · los repos ya aparecen en “nueva app”",
  "system.settings.githubSection": "GitHub App",
  "system.settings.githubDesc":
    "clona repos privados sin PAT · instala la App en cada org para ver sus repos",
  "system.settings.manage": "administrar ↗",
  "system.settings.githubChecking": "consultando GitHub…",
  "system.settings.githubStale":
    "La App anterior ya no existe en GitHub. La configuración local se limpió; crea una nueva.",
  "system.settings.noAppBefore":
    "Sin App conectada. Crea la GitHub App desde aquí y en el paso de instalación elige",
  "system.settings.noAppAfter":
    ". Para repos de una organización, instálala también en esa org.",
  "system.settings.createApp": "crear GitHub App ↗",
  "system.settings.localMode":
    "modo local (localhost): GitHub no entrega webhooks aquí; el auto-deploy usa polling.",
  "system.settings.baseDomainBefore": "define",
  "system.settings.baseDomainAfter":
    "arriba para habilitar el flujo (necesita dominio público con TLS).",
  "system.settings.account": "cuenta",
  "system.settings.app": "app",
  "system.settings.repositories": "repositorios",
  "system.settings.scope": "alcance",
  "system.settings.scopeSelected": "seleccionados",
  "system.settings.scopeMissing": "falta instalar",
  "system.settings.installBefore":
    "App creada. Falta instalarla en tu cuenta y elegir",
  "system.settings.installAfter": ".",
  "system.settings.installAll": "instalar con todos los repos ↗",
  "system.settings.partial": "acceso parcial",
  "system.settings.partialBefore":
    "Los repos privados fuera de la selección fallarán al desplegar. Cambia a",
  "system.settings.partialAfter": "en la instalación.",
  "system.settings.changeAccess": "cambiar acceso ↗",
  "system.settings.fullAccess": "acceso total · repos privados habilitados",
  "system.settings.githubError": "github: {error}",
  "system.settings.keysTitle": "Claves de flota",
  "system.settings.keysDesc":
    "guardar valida antes de persistir · sin valores por defecto inventados",
  "system.settings.colFolio": "folio",
  "system.settings.colKey": "clave",
  "system.settings.colValue": "valor",
  "system.settings.desc.node_bin": "Binario node para presets next/node.",
  "system.settings.desc.python_bin": "Binario python para preset python.",
  "system.settings.desc.base_domain": "Dominio base para sugerir <app>.<base>.",
  "system.settings.desc.port_pool": "Rango de puertos locales (ej 3000-3999).",
  "system.settings.desc.site_name": "Nombre visible en rail y cabecera.",
  "system.settings.desc.quota_reqs":
    "Tope de peticiones 30d para el medidor (0 = sin tope).",
  "system.settings.desc.quota_bytes":
    "Tope de tráfico 30d para el medidor (0 = sin tope).",
  "system.settings.validatedHint": "se valida antes de persistir",
  "system.settings.editTitle": "Editar {key}",
  "system.settings.pathOnDisk": "ruta en disco",
  "system.settings.valueAria": "valor de {key}",
  "system.settings.readOnly": "solo lectura",
  "system.settings.loadingKeys": "cargando claves…",
  "system.settings.secretsNote":
    "Los secretos de entorno no viven aquí: se editan por app en {env}. Este panel solo guarda configuración de flota.",

  // Usuarios
  "system.users.description":
    "Solo admin. Viewer lee, operator muta la flota, admin gestiona usuarios y configuración.",
  "system.users.accounts.one": "{n} cuenta",
  "system.users.accounts.other": "{n} cuentas",
  "system.users.new": "nuevo usuario",
  "system.users.newTitle": "Nuevo usuario",
  "system.users.newDesc":
    "lectura lee · operador muta la flota · administrador gestiona todo · contraseña mín. 12",
  "system.users.user": "usuario",
  "system.users.password": "contraseña",
  "system.users.passwordHint": "mín. 12",
  "system.users.passwordPlaceholder": "mínimo 12 caracteres",
  "system.users.role": "rol",
  "system.users.createdAt": "alta",
  "system.users.you": "tú",
  "system.users.cannotDeleteSelf": "no puedes borrarte a ti mismo",
  "system.users.deleteTitle": "borrar usuario",
  "system.users.emptyTitle": "Sin usuarios",
  "system.users.emptyHint": "Crea el primero con «nuevo usuario».",

  // Acceso
  "system.auth.loginTitle": "Inicia sesión",
  "system.auth.loginDesc":
    "Entra con tu cuenta de rkt. Las cuentas se crean desde la terminal; aquí no hay registro.",
  "system.auth.noAccountBefore": "¿Sin cuenta? Créala con",
  "system.auth.permissions": "permisos: {viewer} · {operator} · {admin}",
  "system.auth.user": "usuario",
  "system.auth.userPlaceholder": "tu usuario",
  "system.auth.password": "contraseña",
  "system.auth.passwordPlaceholder": "tu contraseña",
  "system.auth.signIn": "Entrar",
  "system.auth.setupTitle": "Crea la cuenta de administrador",
  "system.auth.setupDesc":
    "Solo se muestra una vez, mientras no exista ningún usuario. Después podrás añadir más desde Usuarios.",
  "system.auth.setupFooterOne": "cuenta única de arranque",
  "system.auth.setupFooterTwo": "contraseña de 12 caracteres como mínimo",
  "system.auth.passwordMismatch": "las contraseñas no coinciden",
  "system.auth.passwordMinPlaceholder": "12 caracteres como mínimo",
  "system.auth.repeatPassword": "repite la contraseña",
  "system.auth.repeatPasswordPlaceholder": "escríbela otra vez",
  "system.auth.createAccount": "Crear cuenta y entrar",

  // Marco de acceso
  "system.access.panelControl": "panel de control",
  "system.access.headline":
    "Cada app con su puerto, su dominio y un supervisor que la mantiene en marcha.",
  "system.access.subtitle":
    "Despliega, revierte y publica sin editar YAML. Solo mostramos lo que medimos de verdad.",
  "system.access.operateDesc": "despliegues · registros · CI",
  "system.access.serveDesc": "dominios · proxy · entorno",
  "system.access.observeDesc": "tráfico · alertas",
  "system.access.restricted": "acceso restringido · sesión con cookie segura",
  "system.access.noUsers": "sin usuarios, rkt no opera",
  "system.access.panel": "panel",
};
