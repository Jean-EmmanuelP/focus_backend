# Risques et Mitigations

## Matrice des risques

```
        IMPACT
          │
    Élevé │  [3]         [1] [2]
          │
   Moyen  │  [5]    [4]
          │
   Faible │       [6]
          │
          └────────────────────────
              Faible  Moyen  Élevé
                  PROBABILITÉ
```

---

## Risques identifiés

### [1] CRITIQUE: Meta change les règles WhatsApp Business

```
Risque: Meta modifie l'API, augmente les prix, ou restreint les cas d'usage

Probabilité: Moyenne (Meta a historiquement changé ses APIs)
Impact: Élevé (WhatsApp = coeur du produit)

Mitigation:
├── Court terme: Monitorer les annonces Meta
├── Moyen terme: Développer l'app iOS comme alternative
├── Long terme: Multi-canal (Telegram, SMS, iMessage?)
└── Toujours: Ne jamais dépendre à 100% d'une plateforme

Plan B si WhatsApp devient inutilisable:
→ Pivot vers app-first avec notifications push
→ Garder la même UX conversationnelle dans l'app
→ Communication transparente aux users
```

### [2] CRITIQUE: Conversion trial trop faible (<5%)

```
Risque: Les gens essaient mais ne payent pas

Probabilité: Moyenne
Impact: Élevé (pas de business sans conversions)

Signaux d'alerte:
├── Activation <30%
├── NPS <20
├── Feedback: "pas assez de valeur"

Mitigation:
├── Tracker l'activation précisément
├── Interviews users qui ne convertissent pas
├── Itérer sur la proposition de valeur
├── Tester différents prix (9.99€, 12.99€)
└── Améliorer l'onboarding

Plan B:
→ Pivot freemium avec features premium
→ Réduire le prix
→ Changer le modèle (pay-per-use?)
```

### [3] MOYEN: Coûts WhatsApp explosent

```
Risque: Meta augmente significativement les prix API

Probabilité: Faible-Moyenne
Impact: Élevé (marge compressée)

Scénario worst case:
Prix actuel: ~0.05€/conversation
Si x3: 0.15€/conversation → 4.50€/user/mois
Marge passe de 58% à 43%

Mitigation:
├── Monitorer les coûts en temps réel
├── Optimiser le nombre de conversations (batch messages)
├── Garder du buffer dans le pricing
└── Augmenter les prix si nécessaire

Seuil critique: Si coûts >30% du revenue → action immédiate
```

### [4] MOYEN: Churn élevé (>15%/mois)

```
Risque: Les users partent après quelques mois

Probabilité: Moyenne (normal pour apps B2C)
Impact: Moyen (LTV réduite)

Causes possibles:
├── Lassitude de Kai (toujours les mêmes réponses)
├── Objectifs atteints (plus besoin)
├── Concurrence
└── Prix perçu trop élevé

Mitigation:
├── Varier les interactions de Kai
├── Ajouter des features régulièrement
├── Programme de winback automatisé
├── Engagement via crew/social
└── Offres de rétention (discount avant churn)

Métriques à surveiller:
├── Usage 7 derniers jours avant churn
├── Dernier message à Kai
└── Raison du churn (survey)
```

### [5] MOYEN: Templates WhatsApp refusés par Meta

```
Risque: Meta refuse les templates marketing

Probabilité: Moyenne (Meta est strict)
Impact: Moyen (moins de proactivité)

Mitigation:
├── Soumettre plusieurs variantes
├── Suivre les guidelines Meta exactement
├── Commencer par templates "utility" (plus faciles)
├── Avoir un plan B sans templates

Plan B sans templates:
→ Uniquement répondre quand l'user écrit
→ Push notifications app pour proactivité
→ Email pour rappels quotidiens
```

### [6] FAIBLE: Compétition copie le concept

```
Risque: Un concurrent lance un coach WhatsApp similaire

Probabilité: Moyenne-Élevée (si ça marche)
Impact: Faible-Moyen

Pourquoi impact limité:
├── First mover advantage
├── Data/historique user = moat
├── Relation établie avec Kai
└── Itération rapide

Mitigation:
├── Construire la marque Kai
├── Accumuler les données pour personnalisation
├── Itérer vite sur les features
└── Communauté/social comme moat
```

---

## Dépendances critiques

```
┌─────────────────────────────────────────────────────────────┐
│                   DÉPENDANCES EXTERNES                       │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Meta WhatsApp API                                         │
│  ├── Criticité: HAUTE                                      │
│  ├── Alternative: Telegram, SMS, App-only                  │
│  └── Monitoring: Status page + alerting                    │
│                                                             │
│  Google Gemini                                             │
│  ├── Criticité: HAUTE                                      │
│  ├── Alternative: OpenAI, Claude, Mistral                  │
│  └── Monitoring: Latence + erreurs                         │
│                                                             │
│  Stripe                                                    │
│  ├── Criticité: HAUTE                                      │
│  ├── Alternative: Paddle, RevenueCat                       │
│  └── Monitoring: Webhook delivery                          │
│                                                             │
│  Supabase                                                  │
│  ├── Criticité: MOYENNE                                    │
│  ├── Alternative: Postgres direct, PlanetScale             │
│  └── Monitoring: Connection pool + latence                 │
│                                                             │
│  Apple App Store                                           │
│  ├── Criticité: MOYENNE (pour l'app)                       │
│  ├── Alternative: Web app (PWA)                            │
│  └── Monitoring: Review rejections                         │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## Plan de contingence

### Si WhatsApp devient inutilisable

```
Jour 0: Incident
├── Communiquer aux users (email + in-app)
├── Activer mode "app-only"
└── Kai répond dans l'app avec même UX

Semaine 1: Stabilisation
├── Push notifications pour proactivité
├── Améliorer chat in-app
└── Communiquer le plan

Mois 1: Adaptation
├── Évaluer alternatives (Telegram, SMS)
├── Ajuster le pricing si nécessaire
└── Itérer sur l'expérience app
```

### Si conversion <5% après M3

```
Actions immédiates:
├── User research intensif (20+ interviews)
├── Analyser les drop-offs précis
├── A/B test agressif sur pricing et messaging
└── Réduire le trial à 3 jours (urgence)

Si toujours <5% après M6:
├── Pivot vers modèle gratuit + premium
├── Ou focus B2B (entreprises)
├── Ou acqui-hire / fermeture
```

---

## Métriques d'alerte

```
┌─────────────────────────────────────────────────────────────┐
│                      DASHBOARD ALERTES                       │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  🔴 ROUGE (action immédiate)                               │
│  ├── Conversion trial <3%                                  │
│  ├── Churn mensuel >20%                                    │
│  ├── NPS <0                                                │
│  ├── Coûts variables >40% revenue                          │
│  └── WhatsApp API down >4h                                 │
│                                                             │
│  🟡 JAUNE (investigation)                                  │
│  ├── Conversion trial 3-7%                                 │
│  ├── Churn mensuel 12-20%                                  │
│  ├── NPS 0-20                                              │
│  ├── Activation <40%                                       │
│  └── CAC >20€                                              │
│                                                             │
│  🟢 VERT (on track)                                        │
│  ├── Conversion trial >10%                                 │
│  ├── Churn mensuel <8%                                     │
│  ├── NPS >40                                               │
│  ├── Activation >60%                                       │
│  └── CAC <15€                                              │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```
