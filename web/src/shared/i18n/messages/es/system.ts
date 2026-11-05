export const system = {
  // Ajustes
  "system.settings.description":
    "Ajustes generales. Se aplican en caliente salvo los binarios, que requieren reiniciar el servicio.",
  "system.settings.keys.one": "{n} clave",
  "system.settings.keys.other": "{n} claves",
  "system.settings.githubConnected": "GitHub conectado. Los repositorios aparecen en Nueva app.",
  "system.settings.githubSection": "GitHub App",
  "system.settings.githubDesc":
    "Clona repositorios privados con GitHub App. Instala la App en cada organización para ver sus repositorios.",
  "system.settings.manage": "administrar",
  "system.settings.githubChecking": "consultando GitHub…",
  "system.settings.githubStale":
    "La App anterior ya no existe en GitHub. La configuración local se limpió; crea una nueva.",
  "system.settings.noAppBefore":
    "Sin App conectada. Crea la GitHub App desde aquí y en el paso de instalación elige",
  "system.settings.noAppAfter":
    ". Para repos de una organización, instálala también en esa org.",
  "system.settings.createApp": "Crear GitHub App",
  "system.settings.localMode":
    "Modo local (localhost): GitHub no entrega webhooks aquí; el despliegue automático usa sondeo periódico.",
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
    "App creada. Falta instalarla en la cuenta y elegir",
  "system.settings.installAfter": ".",
  "system.settings.installAll": "Instalar con todos los repositorios",
  "system.settings.partial": "acceso parcial",
  "system.settings.partialBefore":
    "Los repositorios privados fuera de la selección fallarán al desplegar. Cambia a",
  "system.settings.partialAfter": "en la instalación.",
  "system.settings.changeAccess": "Cambiar acceso",
  "system.settings.fullAccess": "acceso total · repositorios privados habilitados",
  "system.settings.githubError": "GitHub: {error}",
  "system.settings.keysTitle": "Configuración general",
  "system.settings.keysDesc": "Los cambios se validan antes de guardar.",
  "system.settings.colFolio": "id",
  "system.settings.colKey": "clave",
  "system.settings.colValue": "valor",
  "system.settings.desc.node_bin": "Binario de Node para los presets Next/Node.",
  "system.settings.desc.python_bin": "Binario de Python para el preset Python.",
  "system.settings.desc.base_domain": "Dominio base para sugerir <app>.<base>.",
  "system.settings.desc.port_pool": "Rango de puertos locales (ej 3000-3999).",
  "system.settings.desc.site_name": "Nombre visible en la barra lateral y la cabecera.",
  "system.settings.desc.quota_reqs":
    "Tope de peticiones 30d para el medidor (0 = sin tope).",
  "system.settings.desc.quota_bytes":
    "Tope de tráfico 30d para el medidor (0 = sin tope).",
  "system.settings.validatedHint": "se valida antes de guardar",
  "system.settings.editTitle": "Editar {key}",
  "system.settings.pathOnDisk": "ruta en disco",
  "system.settings.valueAria": "valor de {key}",
  "system.settings.readOnly": "solo lectura",
  "system.settings.loadingKeys": "cargando claves…",
  "system.settings.secretsNote":
    "Los secretos de entorno no viven aquí: se editan por app en {env}. Este panel solo guarda configuración general.",

  // Usuarios
  "system.users.description":
    "Solo administrador. Lectura: consulta; operador: modifica apps; administrador: usuarios y configuración.",
  "system.users.accounts.one": "{n} cuenta",
  "system.users.accounts.other": "{n} cuentas",
  "system.users.new": "nuevo usuario",
  "system.users.newTitle": "Nuevo usuario",
  "system.users.newDesc":
    "lectura: solo lectura; operador: modifica apps; administrador: usuarios y configuración. Contraseña mínima de 12 caracteres.",
  "system.users.user": "usuario",
  "system.users.password": "contraseña",
  "system.users.passwordHint": "mín. 12",
  "system.users.passwordPlaceholder": "mínimo 12 caracteres",
  "system.users.role": "rol",
  "system.users.createdAt": "alta",
  "system.users.you": "tú",
  "system.users.cannotDeleteSelf": "no se puede borrar el propio usuario",
  "system.users.deleteTitle": "borrar usuario",
  "system.users.emptyTitle": "Sin usuarios",
  "system.users.emptyHint": "Crea el primero con «nuevo usuario».",

  // Acceso
  "system.auth.loginTitle": "Acceso",
  "system.auth.loginDesc":
    "Entra con una cuenta local. Las cuentas se crean desde la CLI; no hay registro público.",
  "system.auth.noAccountBefore": "Sin cuenta. Crear con",
  "system.auth.permissions": "permisos: {viewer} · {operator} · {admin}",
  "system.auth.user": "usuario",
  "system.auth.userPlaceholder": "usuario",
  "system.auth.password": "contraseña",
  "system.auth.passwordPlaceholder": "contraseña",
  "system.auth.signIn": "Entrar",
  "system.auth.setupTitle": "Crear cuenta de administrador",
  "system.auth.setupDesc":
    "Solo se muestra una vez, mientras no exista ningún usuario. Después se pueden añadir más desde Usuarios.",
  "system.auth.setupFooterOne": "cuenta inicial",
  "system.auth.setupFooterTwo": "contraseña de 12 caracteres como mínimo",
  "system.auth.passwordMismatch": "las contraseñas no coinciden",
  "system.auth.passwordMinPlaceholder": "12 caracteres como mínimo",
  "system.auth.repeatPassword": "repetir contraseña",
  "system.auth.repeatPasswordPlaceholder": "repetir contraseña",
  "system.auth.createAccount": "Crear cuenta y entrar",

  // Marco de acceso
  "system.access.panelControl": "panel de control",
  "system.access.restricted": "acceso restringido · sesión con cookie segura",
  "system.access.noUsers": "sin usuarios, rkt no opera",
  "system.access.panel": "panel",
};
