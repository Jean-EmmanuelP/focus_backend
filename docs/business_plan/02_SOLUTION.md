# La Solution: Focus

## Vision

> "Un coach de vie dans ta poche qui te connaît,
> te challenge et te célèbre - via WhatsApp."

---

## Architecture produit

```
┌─────────────────────────────────────────────────────────────────┐
│                         UTILISATEUR                              │
└─────────────────────────────┬───────────────────────────────────┘
                              │
              ┌───────────────┴───────────────┐
              ▼                               ▼
┌──────────────────────────┐    ┌──────────────────────────────┐
│      WHATSAPP            │    │         APP iOS              │
│   (Interface primaire)   │    │    (Interface secondaire)    │
├──────────────────────────┤    ├──────────────────────────────┤
│                          │    │                              │
│  Interactions:           │    │  Consultations:              │
│  • Morning check-in      │    │  • Dashboard complet         │
│  • Log focus sessions    │    │  • Graphiques & trends       │
│  • Complete rituals      │    │  • Historique détaillé       │
│  • Evening review        │    │  • Paramètres avancés        │
│  • Chat libre avec Kai   │    │                              │
│  • Rappels proactifs     │    │  Actions avancées:           │
│                          │    │  • Timer focus + app blocker │
│  Usage: 10-20x/jour      │    │  • Planning semaine          │
│  Temps: ~30 sec/inter.   │    │  • Gestion quests/rituels    │
│                          │    │                              │
│                          │    │  Usage: 1-2x/jour            │
│                          │    │  Temps: 2-5 min              │
└──────────────┬───────────┘    └──────────────┬───────────────┘
               │                                │
               └────────────────┬───────────────┘
                                ▼
                    ┌───────────────────────┐
                    │     BACKEND UNIFIÉ    │
                    │  • Données partagées  │
                    │  • IA Kai centralisée │
                    │  • Historique unique  │
                    └───────────────────────┘
```

---

## Kai: Le Coach IA

### Personnalité

```
Kai est ton ami exigeant qui:
- Te tutoie (relation proche)
- Est direct (pas de blabla)
- Te challenge (pas de complaisance)
- Te célèbre (reconnaît les efforts)
- Te comprend (adapte selon le contexte)
```

### Exemples de conversations

**Matin - Check-in:**
```
Kai: Salut! 12 jours de streak. Comment tu te sens ce matin?
User: Fatigué, mal dormi
Kai: Ok, journée adaptée alors. Une priorité réaliste pour aujourd'hui?
User: Finir le rapport
Kai: Parfait. Focus de 45min dessus ce matin. Écris-moi quand c'est fait.
```

**Après-midi - Rappel:**
```
Kai: Hey, 14h. T'as lancé ta session sur le rapport?
User: Pas encore, réunions ce matin
Kai: Compris. C'est le moment. 45min, tu coupes tout. Go?
User: Ok go
Kai: C'est parti. À dans 45min.
```

**Soir - Review:**
```
Kai: 19h. Comment s'est passé le rapport?
User: Fini! Content de moi
Kai: 13 jours de streak. Tu gères. Ta plus grande victoire aujourd'hui?
User: Avoir tenu malgré la fatigue
Kai: C'est ça la discipline. Demain on continue?
User: Oui
Kai: Bonne soirée. Repose-toi bien.
```

---

## Fonctionnalités clés

### 1. Morning Check-in (proactif)

```
Déclencheur: Template WhatsApp à 7h30 (configurable)

Flow:
1. "Bonjour [Nom]! Comment tu te sens?" [Boutons: Super/Moyen/Pas top]
2. User répond → Fenêtre 24h ouverte
3. Kai demande la priorité du jour
4. Kai propose un plan adapté
```

**Valeur:** Démarre la journée avec intention

### 2. Focus Sessions (à la demande)

```
User: "Focus 45min sur le rapport"

Kai:
1. Log la session
2. Associe au quest si mentionné
3. Envoie rappel après 45min
4. Célèbre la complétion
```

**Valeur:** Logger en 3 secondes vs 30 dans une app

### 3. Ritual Tracking (liste interactive)

```
User: "Check mes rituels"

Kai: [Liste interactive]
☐ Méditation (10min)
☐ Lecture (20min)
☐ Sport (30min)

User: Clique sur "Méditation"

Kai: "Méditation validée! 2/3 rituels complétés."
```

**Valeur:** Gamification + feedback immédiat

### 4. Evening Review (proactif)

```
Déclencheur: 21h (configurable)

Kai: "Comment s'est passée ta journée?"
→ Recueille: victoire, blocage, mood
→ Projette: objectif de demain
→ Valide ou non le streak du jour
```

**Valeur:** Réflexion + closure de journée

### 5. Stats & Insights (à la demande)

```
User: "Comment va ma semaine?"

Kai:
📊 Cette semaine:
• Focus: 12h (+2h vs semaine dernière)
• Tâches: 23/28 (82%)
• Rituels: 90% complétion
• Streak: 13 jours

Tu progresses. Continue comme ça.
```

**Valeur:** Feedback sans ouvrir l'app

### 6. Adaptive Coaching (intelligence)

```
Le système détecte:
- Streak en danger → Rappel proactif
- 3 jours de mood bas → Check-in empathique
- Objectif proche → Motivation ciblée
- Inactivité → Re-engagement doux
```

**Valeur:** Personnalisation automatique

---

## App iOS: Le complément

### Quand utiliser l'app?

| Besoin | WhatsApp | App |
|--------|----------|-----|
| Logger une session | ✅ | ✅ |
| Voir stats semaine | ✅ (résumé) | ✅ (détaillé) |
| Créer un nouveau quest | ❌ | ✅ |
| Planning semaine | ❌ | ✅ |
| Timer avec blocage apps | ❌ | ✅ |
| Modifier rituels | ❌ | ✅ |
| Leaderboard crew | ❌ | ✅ |

### Fonctionnalités app exclusives

1. **Timer FireMode**
   - Compte à rebours visuel
   - Blocage d'apps via ScreenTime
   - Live Activity sur lock screen

2. **Planning avancé**
   - Vue semaine
   - Drag & drop tâches
   - Intégration Google Calendar

3. **Analytics détaillés**
   - Graphiques de progression
   - Patterns de productivité
   - Comparaison semaine/mois

4. **Gestion des quests**
   - Création/édition complète
   - Milestones et deadlines
   - Association aux domaines de vie

---

## Différenciation compétitive

| Feature | Focus | Headspace | Todoist | Coach humain |
|---------|-------|-----------|---------|--------------|
| Proactif (vient vers toi) | ✅ | ❌ | ❌ | ✅ |
| Disponible 24/7 | ✅ | ✅ | ✅ | ❌ |
| Personnalisé au contexte | ✅ | ❌ | ❌ | ✅ |
| Conversation naturelle | ✅ | ❌ | ❌ | ✅ |
| Prix accessible | ✅ | ✅ | ✅ | ❌ |
| Friction minimale | ✅ | ❌ | ❌ | ❌ |
| Responsabilisation | ✅ | ❌ | ❌ | ✅ |

---

## Évolutions futures

### Phase 2 (M6-M12)
- Voice notes de Kai (TTS)
- Reconnaissance vocale améliorée
- Intégration calendrier proactive
- Groupes de responsabilisation

### Phase 3 (M12-M18)
- Insights prédictifs (ML)
- Coach spécialisé (fitness, études, etc.)
- API pour intégrations
- Version web
