# Telegram KPI Notifications

## Configuration

### Variables d'environnement
```bash
TELEGRAM_BOT_TOKEN=8591445040:AAFG7vvNSNshP4mvdfmBg6TqQaK2hZd1nPY
TELEGRAM_CHAT_ID=7140861003
ADMIN_SECRET=focus-admin-2024-volta
WEBHOOK_SECRET=focus-webhook-volta-secret
```

### Bot Telegram
- **Nom** : Focus Volta Bot
- **Username** : @focus_volta_bot
- **Lien** : https://t.me/focus_volta_bot
- **Owner** : Jean-Emmanuel (@jperrama)

---

## KPIs Trackés

### Acquisition & Adoption
| Event | Trigger | Message |
|-------|---------|---------|
| `user_signup` | Webhook Supabase `/webhooks/user-created` | 🎉 Nouveau User ! |
| `first_routine_created` | `POST /routines` (si count=1) | 🔄 Première routine créée ! |
| `first_quest_created` | `POST /quests` (si count=1) | 🎯 Première quête créée ! |

### Engagement
| Event | Trigger | Message |
|-------|---------|---------|
| `quest_completed` | `PATCH /quests/{id}` (status=completed) | 🏆 Quête complétée ! |
| `community_post_created` | `POST /community/posts` | 📸 Nouveau post communauté |
| `friend_request_accepted` | `POST /friend-requests/{id}/accept` | 🤝 Nouvelle connexion |

### Monétisation (Referrals)
| Event | Trigger | Message |
|-------|---------|---------|
| `referral_applied` | `POST /referral/apply` | 🔗 Code parrain utilisé ! |
| `referral_activated` | `POST /referral/activate` | 💰 Parrainage activé ! |

### Alertes At-Risk
| Event | Trigger | Message |
|-------|---------|---------|
| `user_inactive_3_days` | Cron `/jobs/telegram/check-inactive` | ⚠️ User inactif 3 jours |
| `user_inactive_7_days` | Cron `/jobs/telegram/check-inactive` | 🚨 User inactif 7 jours ! |

---

## Endpoints

### Admin (protégés par `X-Admin-Secret`)
```bash
# Tester les notifications
curl -X POST https://firelevel-backend.onrender.com/admin/telegram/test \
  -H "X-Admin-Secret: focus-admin-2024-volta"

# Voir les balances referral à payer
curl https://firelevel-backend.onrender.com/admin/referral/balances \
  -H "X-Admin-Secret: focus-admin-2024-volta"

# Marquer un paiement effectué
curl -X POST https://firelevel-backend.onrender.com/admin/referral/mark-paid \
  -H "X-Admin-Secret: focus-admin-2024-volta" \
  -H "Content-Type: application/json" \
  -d '{"user_id": "uuid-here", "amount": 18.00}'
```

### Cron Jobs (protégés par `X-Cron-Secret`)
```bash
# Résumé quotidien (à 20h)
POST /jobs/telegram/daily-summary

# Check users inactifs (à 10h)
POST /jobs/telegram/check-inactive
```

---

## Cron Schedule (à configurer dans Render ou cron-job.org)

| Job | Endpoint | Schedule | Description |
|-----|----------|----------|-------------|
| Daily Summary | `/jobs/telegram/daily-summary` | `0 20 * * *` | Résumé KPIs à 20h |
| Check Inactive | `/jobs/telegram/check-inactive` | `0 10 * * *` | Alertes inactifs à 10h |

---

## Architecture

### Fichiers
```
internal/telegram/
├── service.go      # Service principal, envoi messages
├── handler.go      # Handlers HTTP (daily summary, check inactive)
└── webhook.go      # Webhook pour nouveaux users Supabase
```

### Flow
1. **Event métier** → Handler Go détecte l'action
2. **telegram.Get().Send(Event{...})** → Envoi async en goroutine
3. **Telegram Bot API** → Message envoyé à ton chat

---

## Trigger Supabase (optionnel)

Pour recevoir les notifications de nouveaux users, ajoute ce trigger dans Supabase SQL Editor :

```sql
-- Activer l'extension HTTP
CREATE EXTENSION IF NOT EXISTS http WITH SCHEMA extensions;

-- Fonction de notification
CREATE OR REPLACE FUNCTION notify_new_user()
RETURNS trigger AS $$
BEGIN
  PERFORM extensions.http_post(
    'https://firelevel-backend.onrender.com/webhooks/user-created',
    json_build_object('type', 'INSERT', 'record', row_to_json(NEW))::text,
    'application/json'
  );
  RETURN NEW;
EXCEPTION WHEN OTHERS THEN
  -- Ne pas bloquer l'insertion si le webhook échoue
  RETURN NEW;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Trigger
DROP TRIGGER IF EXISTS on_user_created ON public.users;
CREATE TRIGGER on_user_created
  AFTER INSERT ON public.users
  FOR EACH ROW EXECUTE FUNCTION notify_new_user();
```

---

## Résumé Quotidien (exemple)

```
📊 Résumé Quotidien Firelevel

👥 Utilisateurs
• Nouveaux: 3
• Actifs aujourd'hui: 45

🔥 Streaks
• Streaks cassés: 2
• Level ups: 5

⏱️ Focus
• Sessions: 120
• Minutes: 2400

✅ Routines complétées: 340

📸 Posts communauté: 8

🔗 Parrainages ce mois: 12
```

---

## Troubleshooting

### Pas de messages reçus ?
1. Vérifie que `TELEGRAM_BOT_TOKEN` et `TELEGRAM_CHAT_ID` sont dans Render
2. Teste avec `/admin/telegram/test`
3. Check les logs Render pour erreurs

### Changer de chat/groupe ?
1. Ajoute le bot au nouveau groupe
2. Envoie un message dans le groupe
3. Va sur `https://api.telegram.org/bot<TOKEN>/getUpdates`
4. Copie le nouveau `chat.id` (commence par `-` pour les groupes)
5. Update `TELEGRAM_CHAT_ID` dans Render
