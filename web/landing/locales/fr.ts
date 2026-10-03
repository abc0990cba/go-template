import type { Dictionary } from "./en";

/** Version française. Les termes techniques (RBAC, TOTP, outbox, …) restent
 * en anglais, conformément à l'usage de la documentation francophone. */
export const fr: Dictionary = {
  meta: {
    title: "Go Template — la plateforme Go + React qui se déploie en un seul binaire",
  },
  nav: {
    features: "Fonctionnalités",
    how: "Fonctionnement",
    faq: "FAQ",
    signIn: "Se connecter",
    getStarted: "Commencer",
  },
  lang: {
    label: "Langue",
  },
  hero: {
    badge: "Plateforme open source prête à l'emploi",
    titlePre: "La plateforme Go + React qui se déploie comme",
    titleAccent: "un seul binaire",
    titlePost: "",
    subtitle:
      "Authentification, RBAC, tickets avec validations, tâches de fond, flux d'événements et analytique — une base prête pour la production que vous pouvez cloner, étendre et déployer en quelques minutes.",
    ctaPrimary: "Créer un compte gratuit",
    ctaSecondary: "Explorer la démo",
    checks: [
      "Un conteneur, un port",
      "Sans CORS ni hébergement séparé",
      "Sondes de santé incluses",
    ],
  },
  stats: [
    { value: "1", label: "binaire à déployer — frontend embarqué" },
    { value: "18", label: "domaines fonctionnels prêts à l'emploi" },
    { value: "26", label: "décisions d'architecture documentées" },
    { value: "50+", label: "pages de documentation" },
  ],
  features: {
    title: "Toutes les fonctionnalités d'un vrai produit",
    subtitle:
      "Dix-huit domaines fonctionnels aboutis, pas un hello-world. Cliquez sur une carte pour voir ce qu'elle contient — la plomberie est faite, votre premier commit est une fonctionnalité.",
    more: "Voir le détail",
    dialogClose: "Fermer",
    whatsInside: "Ce qui est inclus",
    items: {
      auth: {
        title: "Authentification et MFA",
        teaser:
          "Sessions cookie, identités OAuth, double authentification TOTP avec codes de récupération, vérification e-mail — complet de bout en bout.",
        description:
          "Tout ce qu'exige un vrai flux de connexion, prêt dès le premier jour : sessions cookie à expiration glissante, liaison d'identités OAuth, inscription TOTP par QR code, codes de récupération hachés à usage unique et indicateur de politique mfa.required qui impose la 2FA à un utilisateur ou à toute une organisation.",
        bullets: [
          "Sessions cookie avec rotation et révocation",
          "Inscription TOTP par QR code et codes de récupération",
          "Vérification e-mail et réinitialisation par jetons à usage unique",
          "Liaison OAuth et politique MFA imposée",
        ],
      },
      rbac: {
        title: "RBAC et politiques",
        teaser:
          "Rôles Casbin avec héritage, matrice de permissions ressource:action et éditeur de politiques en direct.",
        description:
          "Le contrôle d'accès basé sur les rôles avec Casbin : héritage de rôles, multi-tenance par domaines et matrice de permissions « ressource:action » modifiable en direct dans la console d'administration. Chaque requête est revérifiée côté serveur et chaque changement de politique rejoint le journal d'audit.",
        bullets: [
          "Rôles avec héritage et attributions par organisation",
          "Matrice ressource:action avec éditeur de politiques brut",
          "Impersonnation admin avec bannière visible et piste d'audit",
          "Vérifications de droits en cache, invalidées à chaque changement",
        ],
      },
      orgs: {
        title: "Organisations et invitations",
        teaser:
          "Vrais tenants avec membres, accès API par organisation et invitations acceptées en une étape.",
        description:
          "Les domaines Casbin font des organisations de vrais tenants : membres, rôles propres au tenant et accès API par l'en-tête X-Org-Id. Les invitations par e-mail portent des jetons à usage unique et accordent l'adhésion et le rôle de domaine en une seule étape.",
        bullets: [
          "Multi-tenance via domaines Casbin et sélecteur de périmètre",
          "Membres d'organisation avec rôles par tenant",
          "Invitations par e-mail avec jetons à usage unique",
          "Acceptation en une étape : adhésion et rôle dans une transaction",
        ],
      },
      tickets: {
        title: "Tickets et validations",
        teaser:
          "Un flux de travail façon Jira : machine à états, chaînes de validation multi-étapes, vues demandeur et validateur.",
        description:
          "Un flux de travail digne de la production : machine à états des tickets avec des issues distinctes « retiré par le demandeur » et « refusé par le validateur », chaînes de validation multi-étapes configurables par type de ticket et espaces séparés « Mes tickets » et « Validations ».",
        bullets: [
          "Machine à états avec reprise et livraison du résultat",
          "Chaînes multi-étapes configurables par type de ticket",
          "Décisions valider/refuser soumises aux rôles",
          "Interfaces distinctes pour demandeur et validateur",
        ],
      },
      audit: {
        title: "Journal d'audit",
        teaser:
          "Enregistrements immuables qui/quoi/quand avec diffs avant/après, IP et request id.",
        description:
          "Chaque mutation significative écrit un enregistrement d'audit immuable : acteur, action, diff avant/après, adresse IP et request id — écrit dans la même transaction que la modification, avec une vue d'administration filtrable.",
        bullets: [
          "Diffs JSON avant/après pour chaque changement",
          "Corrélation acteur, IP et request id",
          "Vue d'administration avec filtres",
          "Écriture dans la même transaction — aucune entrée manquée",
        ],
      },
      settings: {
        title: "Paramètres et feature flags",
        teaser:
          "Réglages clé-valeur typés en base, interface de gestion, flags publics et cache read-through.",
        description:
          "Un magasin de paramètres typé dans PostgreSQL avec une interface pour le modifier : validation par clé, flags publics lisibles avant connexion et cache read-through pour que les réglages fréquents ne touchent jamais la base à chaque requête.",
        bullets: [
          "Clés typées avec validation",
          "Flags publics pour les pages pré-connexion",
          "Interface de paramètres d'administration",
          "Cache read-through avec invalidation",
        ],
      },
      jobs: {
        title: "Tâches de fond",
        teaser:
          "File d'attente sur Redis (asynq) avec retries, planification et console des tâches en direct.",
        description:
          "Un vrai processus worker sur asynq et Redis : enfilement idempotent, retries avec backoff, tâches différées et périodiques, gestion des échecs et page des tâches avec statuts en direct via Server-Sent Events.",
        bullets: [
          "Processus worker séparé et enfilement idempotent",
          "Retries avec backoff, tâches différées et périodiques",
          "Console de file en direct avec mises à jour SSE",
          "Arrêt gracieux avec propagation du contexte",
        ],
      },
      streams: {
        title: "Flux d'événements et outbox",
        teaser:
          "Consommateurs Kafka et NATS JetStream avec traitement exactement une fois et console de rejeu DLQ.",
        description:
          "Enrichissement d'utilisateurs fusionné depuis deux systèmes externes via Kafka et NATS JetStream : relais outbox transactionnel, déduplication inbox, gardes de séquence par source et propriété des champs verrouillée par ligne — exactement une fois, avec une file de lettres mortes et une console d'administration de rejeu.",
        bullets: [
          "Outbox transactionnel → relais → broker",
          "Déduplication inbox et gardes de séquence (exactement une fois)",
          "File de lettres mortes avec rejeu admin",
          "Adaptateurs Kafka et NATS JetStream",
        ],
      },
      webhooks: {
        title: "Webhooks",
        teaser:
          "Abonnements aux événements avec signatures HMAC-SHA256, retries via file et journal de livraison.",
        description:
          "Abonnez-vous aux événements du domaine et livrez-les n'importe où : charges utiles signées HMAC-SHA256, livraison via file d'attente avec retries et backoff, journal complet des livraisons avec codes de réponse et bouton d'envoi de test par abonnement.",
        bullets: [
          "Signature HMAC-SHA256 à chaque livraison",
          "Retries via file d'attente avec backoff",
          "Journal des livraisons avec codes de réponse",
          "Envoi de test en un clic",
        ],
      },
      notifications: {
        title: "Notifications",
        teaser:
          "Centre de notifications in-app avec badge des non-lus et toasts en direct par SSE.",
        description:
          "Un centre de notifications dans l'application : badge des non-lus, marquage comme lu et toasts en direct poussés par Server-Sent Events — sans polling ni infrastructure au-delà du binaire.",
        bullets: [
          "Centre de notifications avec badge des non-lus",
          "Toasts en direct par Server-Sent Events",
          "Marquer lu et tout marquer",
          "Préférences de notification par utilisateur",
        ],
      },
      apikeys: {
        title: "Clés API et octrois JWT",
        teaser:
          "Jetons porteurs gtpl_ à périmètre réduit, plus octrois JWT façon OAuth avec rotation.",
        description:
          "Deux systèmes de jetons de premier ordre : clés API personnelles (gtpl_…) affichées une seule fois, hachées au repos et révocables, avec réduction de périmètre objet:action — et octrois par mot de passe, MFA et refresh façon OAuth avec rotation et détection de réutilisation pour les clients mobiles et API.",
        bullets: [
          "Clés API affichées une fois, hachées, révocables",
          "Réduction de périmètre (objet:action) par clé",
          "Rotation des refresh avec détection de réutilisation",
          "Octrois par mot de passe, MFA et refresh",
        ],
      },
      files: {
        title: "Stockage de fichiers",
        teaser:
          "Objets S3/MinIO avec sommes de contrôle, téléchargements soumis aux droits et URLs pré-signées.",
        description:
          "Téléversement avec sommes de contrôle vers S3 ou MinIO (en mémoire en développement), téléchargements soumis aux droits, pièces jointes aux tickets et URLs pré-signées — la purge des rétentions est câblée dans le worker.",
        bullets: [
          "Adaptateurs S3, MinIO et en mémoire",
          "Vérification des sommes de contrôle de chaque objet",
          "Téléchargements soumis aux droits",
          "URLs pré-signées pour un accès direct",
        ],
      },
      export: {
        title: "Export de données RGPD",
        teaser:
          "Une tâche en file construit un ZIP de JSON par entité, téléchargement réservé au propriétaire, rétention de 7 jours.",
        description:
          "Un export de données façon RGPD : demandez-le depuis le profil, une tâche en file construit un ZIP de JSON par entité (profil, tickets, audit, fichiers), les liens de téléchargement sont réservés au propriétaire et expirent, et les vieilles archives sont purgées automatiquement.",
        bullets: [
          "Demande en un clic depuis le profil",
          "ZIP de JSON par entité construit par le worker",
          "Liens réservés au propriétaire, avec expiration",
          "Purge automatique après 7 jours",
        ],
      },
      search: {
        title: "Cache et recherche",
        teaser:
          "Cache read-through avec singleflight, ETag/304 sur les listes lourdes, recherche plein texte soumise aux droits.",
        description:
          "Les lectures fréquentes sont servies depuis un cache mémoire ou Redis avec singleflight, pour qu'un manqué sur une clé chaude ne déclenche pas d'avalanche sur le stockage ; les listes lourdes répondent 304 par ETag, et la recherche plein texte sur utilisateurs et tickets est soumise aux droits et classée.",
        bullets: [
          "Cache read-through (mémoire/Redis) avec singleflight",
          "Revalidation ETag / If-None-Match en 304",
          "Recherche plein texte Postgres (tsvector) avec classement",
          "Résultats limités aux droits de l'appelant",
        ],
      },
      resilience: {
        title: "Boîte à outils de résilience",
        teaser:
          "Disjoncteurs, limitation de débit distribuée, clés d'idempotence et cloisons — le tout avec métriques.",
        description:
          "Les modes de défaillance sont traités avant la mise en production : disjoncteurs autour de chaque appel sortant, limitation de débit distribuée sur Redis avec repli en mémoire, middleware Idempotency-Key qui transforme les réessais clients en rejeux de réponse, et cloisons par dépendance — le tout exporté en métriques Prometheus.",
        bullets: [
          "Disjoncteurs avec métriques d'état",
          "Limitation de débit distribuée avec repli mémoire",
          "Middleware de rejeu par Idempotency-Key",
          "Cloisons à concurrence bornée",
        ],
      },
      observability: {
        title: "Observabilité et SLO",
        teaser:
          "Traces OpenTelemetry, métriques RED, alertes burn rate SLO et tableaux de bord livrés en code.",
        description:
          "Logs, métriques et traces avec corrélation trace↔log à chaque frontière — HTTP, tâches, Kafka/NATS, pgx, Redis, ClickHouse — plus métriques RED Prometheus, profilage continu, et tableaux de bord, règles d'alerte et alertes burn rate SLO livrés en code.",
        bullets: [
          "Traces OpenTelemetry à chaque frontière",
          "Métriques RED et statistiques de pool Prometheus",
          "Tableaux de bord et règles d'alerte en code",
          "Alertes burn rate SLO et profilage continu",
        ],
      },
      deploy: {
        title: "Déployez partout",
        teaser: "Un binaire — ou Helm avec canary limité par SLO, manifests GitOps ou Nomad.",
        description:
          "Livrez à votre façon : un seul binaire distroless avec le frontend embarqué ; un chart Helm avec HPA, PDB, sondes et montages de secrets ; un canary via Argo Rollouts limité par les SLO avec retour arrière automatique ; des manifests GitOps ; ou un job Nomad. Les versions sont signées, scannées et attestées SBOM.",
        bullets: [
          "Un binaire statique, frontend inclus",
          "Chart Helm avec canary limité par SLO",
          "Manifests GitOps et alternative Nomad",
          "Versions signées, scannées, attestées SBOM",
        ],
      },
      dx: {
        title: "Expérience développeur",
        teaser:
          "Démos en une commande, jeu de données riche pré-rempli, parcours de régression et génération depuis OpenAPI.",
        description:
          "Le modèle est conçu pour être exploré : make demo-up démarre les brokers, devtools publie de vrais événements, le jeu de données pré-rempli garnit chaque écran au premier lancement, un script de régression parcourt chaque fonctionnalité de bout en bout, et le client API du frontend est généré depuis la spécification OpenAPI.",
        bullets: [
          "Scénarios make demo-* : enrichissement, DLQ, disjoncteurs…",
          "Riche jeu de données de démo au premier lancement",
          "Script de parcours de régression sur chaque fonctionnalité",
          "Génération du client OpenAPI → TypeScript",
        ],
      },
    },
  },
  how: {
    title: "Du clone à la production en trois étapes",
    subtitle: "Aucune semaine de plomberie. Les parties ennuyeuses sont déjà écrites et testées.",
    steps: [
      {
        title: "Cloner et configurer",
        text: "Configuration par variables d'environnement, docker compose pour Postgres et compagnie, jeu de données de démo pré-rempli dès le premier lancement.",
      },
      {
        title: "Étendre le domaine",
        text: "Ajoutez types de tickets, rôles et handlers — les routes par fichiers, l'arbre généré et la spécification OpenAPI suivent automatiquement.",
      },
      {
        title: "Livrer",
        text: "docker compose, Helm ou Nomad. Sondes de santé, métriques, traces et limitation de débit incluses.",
      },
    ],
  },
  stack: {
    title: "Une pile volontairement ennuyeuse et éprouvée",
    subtitle:
      "Des technologies que votre équipe connaît déjà — choisies pour les cinq prochaines années, pas pour le prochain engouement.",
  },
  faq: {
    title: "Questions fréquentes",
    items: [
      {
        q: "Qu'est-ce que Go Template ?",
        a: "Une plateforme de départ prête pour la production : une API Go et une console d'administration React avec authentification, RBAC, tickets, tâches de fond, flux en direct et analytique — tout ce dont un vrai produit interne a besoin dès le premier jour.",
      },
      {
        q: "Faut-il Kubernetes pour l'exécuter ?",
        a: "Non. Un binaire Go statique et Postgres suffisent ; le frontend est embarqué dans le binaire lui-même. Des fichiers docker compose, un chart Helm et un job Nomad sont inclus quand un seul hôte ne suffit plus.",
      },
      {
        q: "Comment le frontend web est-il servi ?",
        a: "L'application React se construit en assets statiques embarqués dans le binaire Go et servis en same-origin — pas de CORS, pas d'hébergement séparé, et la page d'accueil est livrée en HTML pré-rendu pour les robots.",
      },
      {
        q: "Dans quelles langues l'interface est-elle disponible ?",
        a: "Cette page et l'interface de l'application existent en anglais, russe, français et allemand — choisissez dans l'en-tête, le choix est mémorisé.",
      },
      {
        q: "Puis-je l'utiliser comme base d'un vrai produit ?",
        a: "C'est exactement le but. La pile est volontairement ennuyeuse — Go, Postgres, React, Tailwind — et la plomberie (sessions, RBAC, audit, tâches, observabilité) est déjà faite, pour que vous commenciez par ce qui distingue votre produit.",
      },
      {
        q: "Est-ce gratuit ?",
        a: "Oui — clonez, étendez, livrez. Pas de paliers payants, pas de clés de licence, pas de communication à la maison.",
      },
    ],
  },
  cta: {
    title: "Prêt à regarder à l'intérieur ?",
    text: "Créez un compte et parcourez le flux complet — du dépôt d'un ticket à la chaîne de validation — sur les données de démo pré-remplies.",
    button: "Commencer gratuitement",
  },
  footer: {
    product: "Produit",
    app: "Application",
    apiDocs: "Documentation API",
    openapiSpec: "Spécification OpenAPI",
    createAccount: "Créer un compte",
    rights: "© 2026 Go Template",
  },
};
