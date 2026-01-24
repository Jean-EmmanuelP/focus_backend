# Modèle de Pricing

## Principes fondamentaux

```
1. WhatsApp gratuit illimité = NON SOUTENABLE (coûts variables)
2. L'essai doit montrer la vraie valeur
3. La conversion doit être sans friction
4. Le prix doit refléter la valeur d'un coach
```

---

## Structure de pricing

```
┌─────────────────────────────────────────────────────────────────┐
│                                                                 │
│   ESSAI GRATUIT (7 jours)                                      │
│   ─────────────────────────                                    │
│   • WhatsApp illimité avec Kai                                 │
│   • Toutes les fonctionnalités                                 │
│   • Pas de carte bancaire requise                              │
│                                                                 │
│   Objectif: Créer l'habitude en 7 jours                        │
│   Coût pour Focus: ~0.40€/user                                 │
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   APRÈS 7 JOURS → CONVERSION                                   │
│                                                                 │
│   ┌─────────────────────────────────────────────────────────┐  │
│   │                                                         │  │
│   │   FOCUS PRO - 14.99€/mois                              │  │
│   │   ───────────────────────                              │  │
│   │                                                         │  │
│   │   WhatsApp:                                            │  │
│   │   ✅ Conversations illimitées avec Kai                 │  │
│   │   ✅ Morning check-in personnalisé                     │  │
│   │   ✅ Evening review                                    │  │
│   │   ✅ Rappels proactifs                                 │  │
│   │   ✅ Voice messages                                    │  │
│   │                                                         │  │
│   │   App iOS:                                             │  │
│   │   ✅ Dashboard complet                                 │  │
│   │   ✅ Timer FireMode + blocage apps                     │  │
│   │   ✅ Quests illimités                                  │  │
│   │   ✅ Stats & analytics                                 │  │
│   │   ✅ Planning semaine                                  │  │
│   │                                                         │  │
│   │   Option annuelle: 119.99€/an (33% économie)          │  │
│   │                                                         │  │
│   └─────────────────────────────────────────────────────────┘  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Pourquoi un seul tier?

| Multi-tiers | Single tier |
|-------------|-------------|
| Confusion ("lequel choisir?") | Clarté ("c'est ça ou rien") |
| Cannibalisation du tier haut | Pas de cannibalisation |
| Support multiple plans | Support simple |
| Users sur tier bas = faible LTV | Tous les users = même LTV |

**Conclusion:** Un seul tier à 14.99€ maximise la simplicité et la conversion.

---

## Unit Economics

### Coûts variables par user actif

| Poste | Coût/mois | Notes |
|-------|-----------|-------|
| WhatsApp API | 1.65€ | ~30 conversations/mois |
| IA (Gemini) | 0.15€ | ~15 échanges/jour |
| Serveur | 0.03€ | Infra partagée |
| **Total** | **1.83€** | |

### Revenue par user

| Source | Montant |
|--------|---------|
| Prix mensuel | 14.99€ |
| - Commission Apple (30%) | -4.50€ |
| - Coûts variables | -1.83€ |
| **= Marge brute** | **8.66€ (58%)** |

### Pour l'abonnement annuel

| Source | Montant |
|--------|---------|
| Prix annuel | 119.99€ |
| - Commission Apple (15%*) | -18.00€ |
| - Coûts variables (12 mois) | -21.96€ |
| **= Marge brute** | **80.03€ (67%)** |

*Apple prend 15% après la première année d'abonnement

---

## Projections financières

### Hypothèses

| Métrique | Valeur | Justification |
|----------|--------|---------------|
| Trials/mois | 1,000 | Marketing + organique |
| Conversion trial→paid | 12% | Benchmark SaaS B2C |
| Churn mensuel | 8% | Moyenne apps abo |
| % annuel vs mensuel | 30% | Incentive 33% réduction |

### Projection 18 mois

| Mois | Nouveaux | Churn | Total actifs | MRR | Coûts var | Marge |
|------|----------|-------|--------------|-----|-----------|-------|
| M1 | 120 | 0 | 120 | 1,798€ | 220€ | 1,578€ |
| M3 | 120 | 28 | 332 | 4,976€ | 608€ | 4,368€ |
| M6 | 150 | 52 | 698 | 10,460€ | 1,277€ | 9,183€ |
| M9 | 180 | 98 | 1,234 | 18,495€ | 2,258€ | 16,237€ |
| M12 | 200 | 152 | 1,948 | 29,191€ | 3,565€ | 25,626€ |
| M18 | 250 | 312 | 3,876 | 58,082€ | 7,093€ | 50,989€ |

### Graphique simplifié

```
MRR (€)
    │
60k │                                              ╱
    │                                            ╱
50k │                                          ╱
    │                                        ╱
40k │                                      ╱
    │                                    ╱
30k │                              ____╱
    │                        ____╱
20k │                  ____╱
    │            ____╱
10k │      ____╱
    │____╱
    └────────────────────────────────────────────
        M1   M3   M6   M9   M12  M15  M18
```

---

## Stratégie de conversion

### Flow de conversion WhatsApp

```
JOUR 7 du trial:

Kai: "Hey [Nom], ça fait 7 jours qu'on travaille ensemble.

Tu as:
• 🔥 7 jours de streak
• ✅ 18 sessions focus (6h30 total)
• 📈 85% de rituels complétés

Pour continuer avec moi, c'est 14.99€/mois.
Ça te coûte moins qu'un café par jour.

[Bouton: Continuer avec Kai - 14.99€/mois]
[Bouton: Voir ce que j'ai accompli]
[Bouton: Pas maintenant]"
```

### Relances (si "Pas maintenant")

```
J8: Pause des rappels proactifs
    "Je te laisse tranquille. Tu sais où me trouver."

J10: Rappel streak
    "Ta streak de 7 jours est en danger. Un message pour la sauver?"

J14: Dernier essai
    "Ça fait une semaine. Tu me manques. Offre spéciale: -20% premier mois."

J21+: Silence
    Réactivation possible à tout moment
```

---

## Prix psychologiques testés

| Prix | Perception | Conversion attendue |
|------|------------|---------------------|
| 9.99€ | "Cheap, peut-être pas sérieux" | 15% |
| **14.99€** | **"Raisonnable pour un coach"** | **12%** |
| 19.99€ | "Un peu cher, je réfléchis" | 8% |
| 24.99€ | "Cher, besoin de justification" | 5% |

**Choix: 14.99€** - Sweet spot entre volume et marge

---

## Offres spéciales

### Lancement

```
"Fondateur" - Premiers 500 users:
• 9.99€/mois à vie (au lieu de 14.99€)
• Badge "Fondateur" dans l'app
• Accès prioritaire nouvelles features

Objectif: Créer urgence + early adopters fidèles
```

### Réactivation

```
Users churned après 30+ jours:
• "Tu nous manques - 50% sur le premier mois de retour"
• 7.50€ premier mois, puis 14.99€

Coût: Marge réduite 1 mois
Bénéfice: User réactivé potentiellement pour 6+ mois
```

### Parrainage

```
"Invite un ami":
• Parrain: 1 mois gratuit par filleul converti
• Filleul: 7 jours trial + 20% premier mois

Coût: ~10€/acquisition
Bénéfice: CAC très bas, users qualifiés
```

---

## Comparaison concurrentielle

| Service | Prix | Ce que tu obtiens |
|---------|------|-------------------|
| Headspace | 12.99€/mois | Méditations guidées (passif) |
| Calm | 14.99€/mois | Méditations + sleep stories |
| Noom | 59€/mois | Coach + curriculum (lourd) |
| BetterHelp | 70€/semaine | Thérapeute humain |
| Coach privé | 200-500€/mois | 1h/semaine |
| **Focus** | **14.99€/mois** | **Coach IA 24/7 + app complète** |

**Positionnement:** Premium parmi les apps, accessible vs coaching humain.
