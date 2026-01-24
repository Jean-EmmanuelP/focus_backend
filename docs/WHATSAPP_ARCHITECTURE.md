# Architecture WhatsApp + App iOS - Système Dual-Auth

## Vue d'ensemble

```
                    ┌─────────────────────────────────────────────────────────────┐
                    │                         UTILISATEUR                          │
                    └─────────────────────────┬───────────────────────────────────┘
                                              │
                    ┌─────────────────────────┴─────────────────────────┐
                    ▼                                                   ▼
     ┌──────────────────────────────┐               ┌──────────────────────────────┐
     │       WHATSAPP BOT           │               │         APP iOS              │
     │   (Interface principale)     │               │    (Dashboard/Stats)         │
     ├──────────────────────────────┤               ├──────────────────────────────┤
     │ Identification: phone_number │               │ Identification: JWT/Supabase │
     │                              │               │                              │
     │ Features:                    │               │ Features:                    │
     │ • Morning check-in           │               │ • Dashboard stats            │
     │ • Log focus session          │               │ • Graphiques/trends          │
     │ • Complete ritual            │               │ • Timer FireMode + blocking  │
     │ • Evening review             │               │ • Crew/Leaderboard           │
     │ • Quick questions            │               │ • Paramètres avancés         │
     │ • Paiement/upgrade           │               │ • Widget iOS                 │
     │ • Rappels proactifs          │               │ • Live Activities            │
     └──────────────┬───────────────┘               └──────────────┬───────────────┘
                    │                                               │
                    └───────────────────────┬───────────────────────┘
                                            ▼
                    ┌──────────────────────────────────────────────┐
                    │              BACKEND GO (unifié)              │
                    ├──────────────────────────────────────────────┤
                    │                                              │
                    │  ┌─────────────────────────────────────────┐ │
                    │  │         UNIFIED CHAT SERVICE            │ │
                    │  │  • Même logique AI (Kai)                │ │
                    │  │  • Même contexte utilisateur            │ │
                    │  │  • Même historique de conversation      │ │
                    │  └─────────────────────────────────────────┘ │
                    │                                              │
                    │  ┌─────────────────────────────────────────┐ │
                    │  │         USER IDENTITY SERVICE           │ │
                    │  │  • Lookup by user_id (app)              │ │
                    │  │  • Lookup by phone_number (WhatsApp)    │ │
                    │  │  • Phone linking flow                   │ │
                    │  └─────────────────────────────────────────┘ │
                    │                                              │
                    └──────────────────────────────────────────────┘
                                            │
                                            ▼
                    ┌──────────────────────────────────────────────┐
                    │              DATABASE (PostgreSQL)           │
                    ├──────────────────────────────────────────────┤
                    │  users:                                      │
                    │    - id (uuid) ← App auth                    │
                    │    - phone_number (unique) ← WhatsApp auth   │
                    │    - phone_verified (bool)                   │
                    │    - whatsapp_linked_at (timestamp)          │
                    │                                              │
                    │  chat_messages:                              │
                    │    - source: 'app' | 'whatsapp'              │
                    │    - Même table, même historique             │
                    └──────────────────────────────────────────────┘
```

---

## Flux d'identification

### Scénario 1: Utilisateur existant (App → WhatsApp)

```
1. User a déjà l'app Focus avec compte Supabase
2. User envoie "Salut" au bot WhatsApp
3. Bot: "Je ne te reconnais pas. Quel est ton email Focus?"
4. User: "jean@email.com"
5. Backend: Envoie code OTP à l'email
6. Bot: "J'ai envoyé un code à j***@email.com. Quel est le code?"
7. User: "123456"
8. Backend: Vérifie OTP, lie phone_number au user_id
9. Bot: "Parfait Jean! Tu as 12 jours de streak. Comment puis-je t'aider?"
```

### Scénario 2: Nouveau utilisateur (WhatsApp → App)

```
1. Nouveau user envoie "Salut" au bot WhatsApp
2. Bot: "Salut! Je suis Kai, ton coach Focus. Pour commencer, télécharge l'app: [lien]"
3. Bot: "Ou si tu préfères commencer ici, dis-moi ton prénom!"
4. User: "Jean"
5. Backend: Crée un user "pending" avec phone_number uniquement
6. Bot: "Super Jean! Commençons..."
   [Onboarding simplifié via WhatsApp]
7. Plus tard: User peut "upgrader" vers l'app complète
```

### Scénario 3: Utilisateur déjà lié

```
1. User envoie message WhatsApp
2. Backend: Lookup phone_number → trouve user_id
3. Backend: Charge contexte complet (streak, tasks, etc.)
4. Kai répond avec contexte personnalisé
```

---

## Webhook WhatsApp - Structure

### Endpoint
```
POST /webhooks/whatsapp
GET  /webhooks/whatsapp  (verification Meta)
```

### Payload entrant (Meta/WhatsApp Cloud API)
```json
{
  "object": "whatsapp_business_account",
  "entry": [{
    "id": "WHATSAPP_BUSINESS_ACCOUNT_ID",
    "changes": [{
      "value": {
        "messaging_product": "whatsapp",
        "metadata": {
          "display_phone_number": "33612345678",
          "phone_number_id": "PHONE_NUMBER_ID"
        },
        "contacts": [{
          "profile": { "name": "Jean Dupont" },
          "wa_id": "33612345678"
        }],
        "messages": [{
          "from": "33612345678",
          "id": "wamid.xxx",
          "timestamp": "1234567890",
          "type": "text",
          "text": { "body": "Salut Kai!" }
        }]
      },
      "field": "messages"
    }]
  }]
}
```

### Réponse sortante (via WhatsApp Cloud API)
```json
POST https://graph.facebook.com/v18.0/{phone_number_id}/messages

{
  "messaging_product": "whatsapp",
  "recipient_type": "individual",
  "to": "33612345678",
  "type": "text",
  "text": { "body": "Salut Jean! Comment vas-tu aujourd'hui?" }
}
```

---

## Base de données - Migrations

```sql
-- Ajouter phone_number à la table users
ALTER TABLE public.users ADD COLUMN IF NOT EXISTS phone_number text UNIQUE;
ALTER TABLE public.users ADD COLUMN IF NOT EXISTS phone_verified boolean DEFAULT false;
ALTER TABLE public.users ADD COLUMN IF NOT EXISTS whatsapp_linked_at timestamp with time zone;

-- Index pour lookup rapide par phone
CREATE INDEX IF NOT EXISTS idx_users_phone ON public.users(phone_number) WHERE phone_number IS NOT NULL;

-- Ajouter source aux messages chat
ALTER TABLE public.chat_messages ADD COLUMN IF NOT EXISTS source text DEFAULT 'app';
-- 'app' = depuis l'app iOS
-- 'whatsapp' = depuis WhatsApp

-- Table pour les OTP de linking
CREATE TABLE IF NOT EXISTS public.phone_linking_otps (
  id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES auth.users ON DELETE CASCADE,
  phone_number text NOT NULL,
  otp_code text NOT NULL,
  expires_at timestamp with time zone NOT NULL,
  verified boolean DEFAULT false,
  created_at timestamp with time zone DEFAULT now()
);

-- Index pour cleanup des OTP expirés
CREATE INDEX IF NOT EXISTS idx_otp_expires ON public.phone_linking_otps(expires_at);

-- Table pour les users "pending" créés via WhatsApp
CREATE TABLE IF NOT EXISTS public.whatsapp_pending_users (
  id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
  phone_number text UNIQUE NOT NULL,
  display_name text,
  onboarding_step text DEFAULT 'welcome',
  created_at timestamp with time zone DEFAULT now(),
  converted_to_user_id uuid REFERENCES auth.users ON DELETE SET NULL
);
```

---

## Structure du code

```
internal/whatsapp/
├── handler.go           # Webhook HTTP handler
├── service.go           # Business logic
├── models.go            # WhatsApp API types
├── client.go            # WhatsApp Cloud API client
├── templates.go         # Message templates (buttons, lists)
├── identity.go          # Phone ↔ User linking
└── intents.go           # Intent detection & routing

internal/chat/
├── handler.go           # (existant) App chat endpoint
├── service.go           # (nouveau) Unified chat logic
└── ai.go                # (extrait) AI generation logic
```

---

## Intent Detection (WhatsApp)

| Message | Intent | Action |
|---------|--------|--------|
| "Salut", "Hello", "Bonjour" | greeting | Greeting contextuel |
| "J'ai focus 30 min" | log_focus | Créer focus_session |
| "Focus 45 minutes sur [quest]" | start_focus | Log session + link quest |
| "Check meditation" | complete_ritual | Mark routine complete |
| "Comment va ma semaine?" | view_stats | Résumé hebdo |
| "Planifie ma journée" | plan_day | Generate day plan |
| "C'est quoi mes tâches?" | list_tasks | Liste des tâches du jour |
| "Je me sens [humeur]" | log_mood | Log morning mood |
| "Upgrade", "Pro", "Abonnement" | payment | Send Stripe link |
| Autre | ai_chat | Passer à Kai pour réponse IA |

---

## Templates WhatsApp (Interactive)

### Morning Check-in
```json
{
  "type": "interactive",
  "interactive": {
    "type": "button",
    "body": {
      "text": "Bonjour Jean! Comment te sens-tu ce matin?"
    },
    "action": {
      "buttons": [
        { "type": "reply", "reply": { "id": "mood_great", "title": "Super bien!" }},
        { "type": "reply", "reply": { "id": "mood_ok", "title": "Ça va" }},
        { "type": "reply", "reply": { "id": "mood_bad", "title": "Pas top" }}
      ]
    }
  }
}
```

### Ritual Completion
```json
{
  "type": "interactive",
  "interactive": {
    "type": "list",
    "body": {
      "text": "Quels rituels as-tu complétés?"
    },
    "action": {
      "button": "Voir rituels",
      "sections": [{
        "title": "Rituels du matin",
        "rows": [
          { "id": "ritual_meditation", "title": "Méditation", "description": "10 min" },
          { "id": "ritual_workout", "title": "Sport", "description": "30 min" }
        ]
      }]
    }
  }
}
```

---

## Paiement via WhatsApp

### Flow
```
1. User: "Je veux passer Pro"
2. Bot: Envoie lien Stripe Checkout avec metadata (user_id, phone)
3. User clique → Stripe Checkout → Paiement
4. Webhook Stripe → Backend met à jour subscription
5. Bot: "Félicitations! Tu es maintenant Pro. Accès illimité débloqué."
```

### Message avec bouton paiement
```json
{
  "type": "interactive",
  "interactive": {
    "type": "cta_url",
    "body": {
      "text": "Focus Pro: 9.99€/mois\n• Quests illimités\n• Stats avancées\n• Priorité support"
    },
    "action": {
      "name": "cta_url",
      "parameters": {
        "display_text": "Passer Pro",
        "url": "https://checkout.stripe.com/c/pay/xxx"
      }
    }
  }
}
```

---

## Variables d'environnement

```bash
# WhatsApp Cloud API
WHATSAPP_PHONE_NUMBER_ID=123456789
WHATSAPP_ACCESS_TOKEN=EAAxxxxx
WHATSAPP_VERIFY_TOKEN=focus_webhook_verify_2024
WHATSAPP_BUSINESS_ACCOUNT_ID=987654321

# Stripe
STRIPE_SECRET_KEY=sk_live_xxx
STRIPE_WEBHOOK_SECRET=whsec_xxx
STRIPE_PRO_PRICE_ID=price_xxx
```

---

## Avantages de cette architecture

1. **Un seul user_id** - Pas de duplication de comptes
2. **Historique unifié** - Conversations WhatsApp + App dans la même table
3. **Contexte partagé** - Kai connaît les stats peu importe la source
4. **Migration fluide** - User peut commencer sur WhatsApp, continuer sur l'app
5. **Scalable** - WhatsApp peut être désactivé/activé sans impact sur l'app
