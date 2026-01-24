# Requirements Techniques

## Architecture globale

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              USERS                                       │
└───────────────────────────────┬─────────────────────────────────────────┘
                                │
                ┌───────────────┴───────────────┐
                ▼                               ▼
┌───────────────────────────┐     ┌───────────────────────────────────────┐
│      WhatsApp Cloud       │     │            App iOS                     │
│      (Meta API)           │     │         (Swift/SwiftUI)                │
└─────────────┬─────────────┘     └──────────────────┬────────────────────┘
              │                                       │
              │ Webhook                               │ REST API
              ▼                                       ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         BACKEND GO (Chi Router)                          │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌─────────────┐ │
│  │   WhatsApp   │  │     Chat     │  │   Identity   │  │  Subscription│ │
│  │   Handler    │  │   Service    │  │   Service    │  │   Service    │ │
│  └──────────────┘  └──────────────┘  └──────────────┘  └─────────────┘ │
│                                                                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌─────────────┐ │
│  │    Focus     │  │   Routines   │  │    Stats     │  │    Quests   │ │
│  │   Sessions   │  │              │  │              │  │             │ │
│  └──────────────┘  └──────────────┘  └──────────────┘  └─────────────┘ │
│                                                                         │
└─────────────────────────────────┬───────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         SERVICES EXTERNES                                │
├───────────────┬───────────────┬───────────────┬─────────────────────────┤
│  PostgreSQL   │  Gemini AI    │    Stripe     │     Meta WhatsApp       │
│  (Supabase)   │  (Google)     │   Payments    │      Cloud API          │
└───────────────┴───────────────┴───────────────┴─────────────────────────┘
```

---

## Composants à implémenter

### 1. Système de Trial & Subscription

```
┌─────────────────────────────────────────────────────────────┐
│                    SUBSCRIPTION STATES                       │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  [NEW_USER] ──► [TRIAL_ACTIVE] ──► [TRIAL_EXPIRED]         │
│                       │                   │                 │
│                       │                   ▼                 │
│                       │            [CHURNED]                │
│                       │                   │                 │
│                       ▼                   │                 │
│               [SUBSCRIBED] ◄──────────────┘                │
│                       │                                     │
│                       ▼                                     │
│               [CANCELLED] ──► [CHURNED]                    │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

**Table subscriptions:**
```sql
CREATE TABLE public.subscriptions (
  id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES auth.users ON DELETE CASCADE,

  -- Status
  status text NOT NULL DEFAULT 'trial',  -- trial, active, cancelled, expired

  -- Trial
  trial_started_at timestamp with time zone DEFAULT now(),
  trial_ends_at timestamp with time zone DEFAULT (now() + interval '7 days'),

  -- Subscription
  stripe_customer_id text,
  stripe_subscription_id text,
  plan_type text,  -- monthly, yearly
  current_period_start timestamp with time zone,
  current_period_end timestamp with time zone,

  -- Metadata
  source text DEFAULT 'whatsapp',  -- whatsapp, app, web
  created_at timestamp with time zone DEFAULT now(),
  updated_at timestamp with time zone DEFAULT now(),

  UNIQUE(user_id)
);
```

### 2. WhatsApp Templates (Meta Approval)

**Templates requis:**

```
1. morning_checkin (Marketing)
   "Bonjour {{1}}! Comment te sens-tu ce matin?"
   Boutons: [Super ☀️] [Moyen 😐] [Pas top 😔]

2. streak_danger (Utility)
   "⚠️ {{1}}, ta streak de {{2}} jours est en danger!
    Un petit focus pour la sauver?"
   Boutons: [Focus maintenant] [Plus tard]

3. evening_review (Marketing)
   "Bonsoir {{1}}! Comment s'est passée ta journée?"

4. trial_ending (Utility)
   "{{1}}, ton essai Focus se termine demain.
    Tu as accompli {{2}} sessions et {{3}} jours de streak.
    Continue avec Kai pour 14.99€/mois."
   Boutons: [Continuer] [Pas maintenant]

5. trial_expired (Utility)
   "{{1}}, ton essai est terminé.
    Tes stats sont sauvegardées. Reprends quand tu veux."
   Boutons: [Reprendre - 14.99€/mois]

6. welcome_back (Marketing)
   "{{1}}! Ça fait {{2}} jours. Tu me manques.
    Offre spéciale: -20% ce mois-ci."
   Boutons: [Reprendre] [Pas maintenant]
```

### 3. Stripe Integration

**Endpoints requis:**

```go
// Créer checkout session
POST /payments/checkout
{
  "plan": "monthly" | "yearly",
  "source": "whatsapp" | "app"
}
Returns: { "checkout_url": "https://checkout.stripe.com/..." }

// Webhook Stripe
POST /webhooks/stripe
Events handled:
- checkout.session.completed
- customer.subscription.created
- customer.subscription.updated
- customer.subscription.deleted
- invoice.payment_failed

// Portail client (gérer abo)
POST /payments/portal
Returns: { "portal_url": "https://billing.stripe.com/..." }

// Status
GET /subscription/status
Returns: {
  "status": "trial" | "active" | "expired",
  "trial_ends_at": "2024-01-20T00:00:00Z",
  "plan": "monthly",
  "current_period_end": "2024-02-15T00:00:00Z"
}
```

### 4. Cron Jobs

**Jobs quotidiens:**

```
┌─────────────────────────────────────────────────────────────┐
│  06:00 UTC - Check trial expirations                        │
│  ─────────────────────────────────────                      │
│  Pour chaque user où trial_ends_at < now() AND status=trial:│
│  → Update status = 'expired'                                │
│  → Envoyer template "trial_expired"                         │
│  → Désactiver rappels proactifs                             │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│  07:30 UTC (Paris 8h30) - Morning check-ins                 │
│  ───────────────────────────────────────                    │
│  Pour chaque user actif (trial ou subscribed):              │
│  → Envoyer template "morning_checkin"                       │
│  → Respecter timezone user si configuré                     │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│  20:00 UTC (Paris 21h) - Evening reviews                    │
│  ────────────────────────────────────                       │
│  Pour chaque user actif:                                    │
│  → Envoyer template "evening_review"                        │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│  22:00 UTC - Streak danger alerts                           │
│  ───────────────────────────────                            │
│  Pour chaque user actif sans activité aujourd'hui:          │
│  → Envoyer template "streak_danger"                         │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│  06:00 UTC - Trial ending reminders (J6)                    │
│  ──────────────────────────────────────                     │
│  Pour chaque user où trial_ends_at entre J+1 et J+2:        │
│  → Envoyer template "trial_ending"                          │
└─────────────────────────────────────────────────────────────┘
```

---

## Variables d'environnement

```bash
# ============================================
# DATABASE
# ============================================
DATABASE_URL=postgresql://user:pass@host:5432/db
SUPABASE_URL=https://xxx.supabase.co
SUPABASE_KEY=xxx
SUPABASE_JWT_SECRET=xxx

# ============================================
# WHATSAPP (Meta Cloud API)
# ============================================
WHATSAPP_PHONE_NUMBER_ID=123456789
WHATSAPP_ACCESS_TOKEN=EAAxxxx
WHATSAPP_VERIFY_TOKEN=focus_webhook_2024
WHATSAPP_BUSINESS_ACCOUNT_ID=987654321

# ============================================
# AI (Google Gemini)
# ============================================
GEMINI_API_KEY=AIzaxxxx

# ============================================
# PAYMENTS (Stripe)
# ============================================
STRIPE_SECRET_KEY=sk_live_xxxx
STRIPE_WEBHOOK_SECRET=whsec_xxxx
STRIPE_MONTHLY_PRICE_ID=price_xxxx
STRIPE_YEARLY_PRICE_ID=price_xxxx

# ============================================
# APP
# ============================================
APP_URL=https://api.focus-app.com
FRONTEND_URL=https://focus-app.com
ENVIRONMENT=production  # development, staging, production

# ============================================
# CRON
# ============================================
CRON_SECRET=xxxx

# ============================================
# NOTIFICATIONS
# ============================================
TELEGRAM_BOT_TOKEN=xxxx  # Pour alertes admin
TELEGRAM_CHAT_ID=xxxx
```

---

## Checklist de lancement

### Backend
- [ ] Subscription service implémenté
- [ ] Stripe webhooks configurés
- [ ] Trial logic fonctionnelle
- [ ] Templates WhatsApp soumis à Meta
- [ ] Templates approuvés par Meta
- [ ] Cron jobs configurés
- [ ] Monitoring/alerting en place

### WhatsApp
- [ ] Meta Business Account vérifié
- [ ] Numéro WhatsApp Business configuré
- [ ] Webhook URL publique et SSL
- [ ] Templates créés et approuvés
- [ ] Test complet du flow

### Stripe
- [ ] Compte Stripe activé
- [ ] Products/Prices créés
- [ ] Webhook endpoint configuré
- [ ] Test payment flow
- [ ] Apple/Google pay activé (optionnel)

### iOS App
- [ ] Subscription status check
- [ ] Deep link vers checkout
- [ ] Paywall si trial expiré
- [ ] Gestion état offline

### Legal
- [ ] CGV mises à jour
- [ ] Politique de confidentialité (WhatsApp data)
- [ ] Mentions RGPD
- [ ] Politique de remboursement
