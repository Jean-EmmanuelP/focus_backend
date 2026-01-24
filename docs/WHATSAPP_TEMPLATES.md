# WhatsApp Templates - Meta Approval Guide

## Overview

This document describes the WhatsApp message templates required for Focus app's proactive notifications. All templates must be approved by Meta before use.

## Template Categories

Meta supports 3 template categories:

| Category | Description | Delivery Rate | Use Case |
|----------|-------------|---------------|----------|
| **UTILITY** | Transactional/updates | High | Check-ins, streak alerts, reminders |
| **MARKETING** | Promotional | Medium | Re-engagement, offers |
| **AUTHENTICATION** | OTP/verification | Highest | Phone verification |

## Templates to Submit

### 1. Morning Check-in (`morning_checkin_v1`)

**Category**: UTILITY
**Language**: French (fr)
**Purpose**: Daily morning engagement to help users plan their day

```
Body: Salut {{1}} ! ☀️ C'est l'heure de planifier ta journée. Comment tu te sens ce matin ?

Buttons:
- Super motivé 🔥 (Quick Reply)
- Ça va 👍 (Quick Reply)
- Fatigué 😴 (Quick Reply)
```

**Example Values**:
- {{1}}: Marie

---

### 2. Streak Danger Alert (`streak_danger_v1`)

**Category**: UTILITY
**Language**: French (fr)
**Purpose**: Warn users their streak is at risk

```
Body: Hey {{1}} ! 🔥 Ta streak de {{2}} jours est en danger ! Une petite session de 5 minutes suffit pour la sauver. Tu veux qu'on s'y mette ?

Buttons:
- Go session 5min 🚀 (Quick Reply)
- Plus tard (Quick Reply)
```

**Example Values**:
- {{1}}: Marie
- {{2}}: 14

---

### 3. Evening Review (`evening_review_v1`)

**Category**: UTILITY
**Language**: French (fr)
**Purpose**: Daily evening reflection prompt

```
Body: Bonne soirée {{1}} ! 🌙 Tu as focus {{2}} minutes aujourd'hui et complété {{3}} tâches. Comment s'est passée ta journée ?

Buttons:
- Super journée 🎉 (Quick Reply)
- Correct 👍 (Quick Reply)
- Difficile 😔 (Quick Reply)
```

**Example Values**:
- {{1}}: Marie
- {{2}}: 45
- {{3}}: 3

---

### 4. Streak Milestone (`streak_milestone_v1`)

**Category**: UTILITY
**Language**: French (fr)
**Purpose**: Celebrate streak achievements

```
Body: 🔥🔥🔥 {{1}} JOURS DE STREAK ! {{2}}, tu es officiellement {{3}} ! Continue comme ça, c'est énorme 💪

No buttons (celebration only)
```

**Example Values**:
- {{1}}: 30
- {{2}}: Marie
- {{3}}: en feu

**Streak Level Descriptions**:
- 7 days: "en flammes"
- 14 days: "en feu"
- 30 days: "un phoenix"
- 60 days: "une supernova"
- 100+ days: "légendaire"

---

### 5. Quest Deadline (`quest_deadline_v1`)

**Category**: UTILITY
**Language**: French (fr)
**Purpose**: Remind users of approaching quest deadlines

```
Body: ⏰ Hey {{1}} ! Ta quest "{{2}}" arrive à échéance dans {{3}} jours. Tu es à {{4}}% - on s'y met ?

Buttons:
- C'est parti 🎯 (Quick Reply)
- Voir mes quests (Quick Reply)
```

**Example Values**:
- {{1}}: Marie
- {{2}}: Lire 10 livres
- {{3}}: 3
- {{4}}: 70

---

### 6. Trial Ending (`trial_ending_v1`)

**Category**: UTILITY
**Language**: French (fr)
**Purpose**: Notify users when their trial is about to expire

```
Body: Hey {{1}} ! 👋 Ton essai gratuit se termine dans {{2}} jours. Tu as déjà accumulé {{3}} jours de streak ! Pour continuer avec Kai, passe en premium.

Buttons:
- Passer Premium ⭐ (URL: https://focus.app/subscribe?user={{1}})
- Plus tard (Quick Reply)
```

**Example Values**:
- {{1}}: Marie (also used in URL)
- {{2}}: 2
- {{3}}: 5

---

### 7. Welcome (`welcome_v1`)

**Category**: UTILITY
**Language**: French (fr)
**Purpose**: Onboard new users who link their WhatsApp

```
Body: Hey ! 👋 Moi c'est Kai, ton compagnon de productivité. Je suis là pour t'aider à rester focus et atteindre tes objectifs. On commence ?

Buttons:
- C'est parti ! 🚀 (Quick Reply)
- C'est quoi Focus ? (Quick Reply)
```

No variables in this template.

---

### 8. Phone Verification (`phone_verification_v1`)

**Category**: AUTHENTICATION
**Language**: French (fr)
**Purpose**: Send OTP codes for phone verification

```
Body: Ton code de vérification Focus est : {{1}}. Il expire dans 10 minutes.

No buttons (OTP only)
```

**Example Values**:
- {{1}}: 123456

---

### 9. Inactivity Reminder (`inactivity_reminder_v1`)

**Category**: UTILITY
**Language**: French (fr)
**Purpose**: Re-engage inactive users

```
Body: Hey {{1}} ! Ça fait {{2}} jours qu'on ne s'est pas parlé. Tout va bien ? Je suis là si tu veux reprendre 💪

Buttons:
- Je reprends 🔥 (Quick Reply)
- J'ai besoin d'une pause (Quick Reply)
```

**Example Values**:
- {{1}}: Marie
- {{2}}: 5

---

## Meta Business Manager Setup

### Prerequisites

1. **Meta Business Account**: Create at [business.facebook.com](https://business.facebook.com)
2. **WhatsApp Business Account (WABA)**: Linked to your business
3. **Phone Number**: Verified business phone number
4. **Business Verification**: Complete Meta business verification

### Steps to Submit Templates

1. Go to [Meta Business Suite](https://business.facebook.com)
2. Navigate to **WhatsApp Manager** > **Account tools** > **Message templates**
3. Click **Create template**
4. Fill in:
   - **Name**: Use exact names from this doc (e.g., `morning_checkin_v1`)
   - **Category**: Select appropriate category
   - **Language**: French (fr)
   - **Header** (optional): None for our templates
   - **Body**: Copy text with {{1}}, {{2}} placeholders
   - **Footer** (optional): None for our templates
   - **Buttons**: Add as specified

5. Provide **Sample Content** for each variable
6. Submit for review

### Approval Timeline

- **UTILITY templates**: Usually 24-48 hours
- **MARKETING templates**: 1-3 business days
- **AUTHENTICATION templates**: Usually within hours

### Common Rejection Reasons

1. **Variable placement**: Variables must have clear context
2. **Promotional content**: UTILITY templates should not be promotional
3. **Missing examples**: All variables need realistic examples
4. **Policy violations**: No spam, misleading content, or prohibited content

---

## Environment Variables

```bash
# WhatsApp Cloud API
WHATSAPP_PHONE_NUMBER_ID=your_phone_number_id
WHATSAPP_ACCESS_TOKEN=your_access_token
WHATSAPP_BUSINESS_ACCOUNT_ID=your_waba_id
WHATSAPP_WEBHOOK_VERIFY_TOKEN=your_webhook_token

# Meta App
META_APP_ID=your_app_id
META_APP_SECRET=your_app_secret
```

---

## API Usage

### Sending a Template

```go
// Using TemplateService
templateSvc := whatsapp.NewTemplateService(client)

// Morning check-in
err := templateSvc.SendMorningCheckIn("+33612345678", "Marie")

// Streak danger
err := templateSvc.SendStreakDanger("+33612345678", "Marie", 14)

// Evening review
err := templateSvc.SendEveningReview("+33612345678", "Marie", 45, 3)
```

### Getting All Templates (for debugging)

```go
templates := whatsapp.GetAllTemplates()
for _, t := range templates {
    fmt.Printf("Template: %s (%s)\n", t.Name, t.Category)
}
```

---

## Cron Jobs

| Job | Schedule | Template | Description |
|-----|----------|----------|-------------|
| Morning Check-in | 7-9 AM local | `morning_checkin_v1` | Based on user timezone |
| Streak Danger | 6 PM local | `streak_danger_v1` | Users with no activity today |
| Evening Review | 9 PM local | `evening_review_v1` | Users who completed check-in |
| Inactivity | Daily 10 AM | `inactivity_reminder_v1` | Users inactive 3+ days |

---

## Testing

### Test Phone Numbers

Meta provides test phone numbers for development:
1. Go to WhatsApp Manager > Phone numbers
2. Add a test phone number
3. Use for sending test messages without approval

### Sandbox Mode

Before templates are approved:
- Can only send to registered test numbers
- Templates must be pre-approved for production use

---

## Localization (Future)

Templates are currently French-only. For multi-language support:

1. Create duplicate templates with language suffix (e.g., `morning_checkin_v1_en`)
2. Submit each language variant for approval
3. Store user language preference
4. Route to correct template based on preference

Planned languages:
- French (fr) - Current
- English (en) - Priority
- Spanish (es) - Future
