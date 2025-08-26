# HappyCow Automatic Vendor Validation System

## Project Overview
MVP system to automatically validate and approve restaurant venues for HappyCow directory using AI scoring and Google Maps verification. The system replicates the current manual validation process shown in the HappyCow admin interface to reduce manual review workload by 80%.

## Current Status & Problem
- **56 Pending venues** in Korea
- **54 Pending venues** in China
- **7000+ total venues** pending manual review
- Current process: Manual comparison of submitted data vs Google Places data
- **Target**: Reduce manual review by 80% (under 1000 venues requiring human review)

## Technical Architecture

### Core Technology Stack
- **Backend**: Go-based application with concurrent processing
- **APIs**: Google Maps/Places API for venue verification
- **AI**: OpenAI API for intelligent scoring (85/100 auto-approval threshold)
- **Database**: MySQL (existing HappyCow venues table)
- **Interface**: Simple web-based admin interface
- **Deployment**: Single binary behind existing VPN & Nginx setup

### Performance Requirements
- Process 7000+ venues in hours (vs weeks manually)
- Handle hundreds of venues concurrently
- API cost budget: Under $200
- Development timeline: 2 days

## Database Schema

### Related Table Structures
```sql
-- Main venues table
create table happycow.venues
(
    id                         bigint auto_increment primary key,
    name                       varchar(255)                null,
    location                   varchar(255)                null,
    phone                      varchar(35)                 null,
    url                        varchar(255)                null,
    lat                        double(12, 6)               null,
    lng                        double(12, 6)               null,
    zipcode                    varchar(20)                 null,
    openhours                  text                        null,
    openhours_note             varchar(512)                null,
    timezone                   varchar(255)                null,
    additionalinfo             text                        null,
    
    -- Status fields
    active                     int default 0               null, -- 0=pending, 1=approved, -1=rejected
    vegan                      int default 0               not null,
    vegonly                    int default 1               null,
    
    -- Metadata
    user_id                    int unsigned                not null, -- FK to members.id
    created_at                 datetime                    null,
    date_added                 datetime                    null,
    date_updated               datetime                    null,
    admin_last_update          datetime                    null,
    admin_note                 text                        null,
    
    -- Other fields...
    entrytype, fburl, instagram_url, other_food_type, price, vdetails,
    hash, email, ownername, sentby, sponsor_level, crossstreet,
    geolocation, admin_hold, admin_hold_email_note, updated_by_id,
    made_active_by_id, made_active_at, show_premium, category,
    pretty_url, edit_lock, request_vegan_decal_at, 
    request_excellent_decal_at, source, path
);

-- User information table
create table happycow.members
(
    id                int auto_increment primary key,
    username          varchar(21)  default ''                not null,
    veg_status        int          default 1                 not null,
    birthdate         int          default 0                 not null,
    email             varchar(150) default ''                not null,
    status            int          default 0                 not null,
    trusted           tinyint(1)   default 0                 not null,
    contributions     int          default 0                 not null,
    -- Other fields...
);

-- Venue ownership/admin relationship
create table happycow.venue_admin
(
    id          int auto_increment primary key,
    venue_id    int      not null, -- FK to venues.id
    user_id     int      not null, -- FK to members.id
    created_at  datetime not null,
    last_viewed datetime null
);

-- Ambassador status and points
create table happycow.ambassadors
(
    id      int unsigned auto_increment primary key,
    user_id int          null, -- FK to members.id
    path    varchar(128) null, -- geographic region/country
    level   int          null,
    points  int unsigned null
);
```

### Key Fields for Validation
- `name` - Restaurant name
- `location` - Street address
- `phone` - Contact phone number
- `lat`, `lng` - Coordinates
- `url` - Website URL
- `openhours` - Operating hours
- `zipcode` - Postal code
- `active` - Approval status (0=pending, 1=approved, -1=rejected)
- `user_id` - Submitter ID (FK to members.id)

## User Authority System

### User Classification (Priority Order)
1. **Venue Owner/Admin** (`venue_admin` table)
    - Has administrative rights to the specific venue
    - Highest authority for venue data
    - Auto-approve if no missing data

2. **High-Ranking Ambassador** (`ambassadors` table)
    - Ambassador in same geographic region (`path` field)
    - High points/level within the region
    - Trusted authority for regional venues

3. **Trusted Members** (`members` table)
    - `trusted = 1` flag set
    - Reliable but lower authority than venue admins/ambassadors

4. **Regular Members**
    - Standard validation applies

### Authority-Based Validation Rules

#### 1. Venue Owner/Admin Submissions
- **IF** user is in `venue_admin` table for this venue
- **AND** no critical data is missing
- **THEN** auto-approve (bypass Google verification)
- **TRUST** user data over Google data when conflicts exist
- **FILL** missing data from Google if available

#### 2. High-Ranking Ambassador Submissions
- **IF** user is ambassador in same geographic region
- **AND** ambassador has high points/level (threshold TBD)
- **AND** no critical data is missing
- **THEN** auto-approve with minimal verification
- **TRUST** user data over Google data for non-critical fields
- **FILL** missing data from Google if available

#### 3. Standard Validation
- Apply full scoring system for regular members
- Use Google data as authoritative source
- Require 85+ points for auto-approval

## Validation Workflow

### Enhanced Process Flow
1. **User Authority Check**: Determine submitter's authority level
2. **Data Completeness Check**: Identify missing critical fields
3. **Google Data Enhancement**: Fill missing data from Google Places
4. **Authority-Based Decision**:
    - Venue Owner + Complete Data = Auto-approve
    - High Ambassador + Complete Data = Auto-approve
    - Others = Apply scoring system
5. **Conflict Resolution**: Trust authority user data over Google when conflicts exist

### Data Comparison Fields
| Field | HappyCow Source | Google Places Source | Weight | Critical? |
|-------|----------------|---------------------|---------|-----------|
| **Venue Name** | `name` | `name` | 25 points | ✅ Yes |
| **Street Address** | `location` | `formatted_address` | 20 points | ✅ Yes |
| **Phone Number** | `phone` | `formatted_phone_number` | 10 points | ❌ No |
| **Geolocation** | `lat`, `lng` | `geometry.location` | 15 points | ✅ Yes |
| **Website** | `url` | `website` | 5 points | ❌ No |
| **Opening Hours** | `openhours` | `opening_hours` | 10 points | ❌ No |
| **Business Status** | N/A | `business_status` | 5 points | ❌ No |
| **Postal Code** | `zipcode` | `address_components` | 5 points | ❌ No |
| **Vegan Relevance** | `additionalinfo`, `vdetails` | AI analysis | 5 points | ✅ Yes |

### Critical Data Requirements
**Must have for auto-approval**:
- Venue name (`name`)
- Street address (`location`)
- Geolocation (`lat`, `lng`)
- Vegan/vegetarian relevance indicators

## Scoring System (0-100 points)

### Authority Modifiers
- **Venue Owner/Admin**: +50 bonus points (near-automatic approval)
- **High-Ranking Ambassador**: +30 bonus points
- **Trusted Member**: +10 bonus points
- **Regular Member**: No bonus

### Base Scoring (applies after authority bonuses)

#### 1. Venue Name Matching (25 points)
- **Exact match**: 25 points
- **Very close match** (minor spelling/formatting): 20 points
- **Partial match** (missing/extra words): 15 points
- **Different but identifiable**: 10 points
- **No match/unidentifiable**: 0 points

#### 2. Address Accuracy (20 points)
- **Exact street address match**: 20 points
- **Same street, minor number difference**: 15 points
- **Same area/block**: 10 points
- **Different street but same neighborhood**: 5 points
- **Completely different**: 0 points

#### 3. Geolocation Accuracy (15 points)
- **Within 10m**: 15 points
- **Within 50m**: 12 points (acceptable per current system)
- **Within 100m**: 8 points
- **Within 500m**: 5 points
- **Over 500m**: 0 points

#### 4. Phone Number Verification (10 points)
- **Exact match**: 10 points
- **Same number, different formatting**: 8 points
- **No phone provided but Google has one**: 5 points
- **Different numbers**: 0 points
- **No phone data available**: 5 points (neutral)

#### 5. Website Verification (5 points)
- **Exact URL match**: 5 points
- **Same domain, different page**: 4 points
- **No website provided**: 2 points (neutral)
- **Different website**: 0 points

#### 6. Business Hours Verification (10 points)
- **Hours match exactly**: 10 points
- **Minor differences** (within 30 min tolerance): 8 points
- **Different but reasonable**: 5 points
- **Significantly different**: 2 points
- **No hours data**: 3 points (neutral)

#### 7. Business Status (5 points)
- **Google shows "OPERATIONAL"**: 5 points
- **Google shows "TEMPORARILY_CLOSED"**: 2 points
- **Google shows "PERMANENTLY_CLOSED"**: 0 points
- **Unknown status**: 2 points

#### 8. Postal Code (5 points)
- **Exact match**: 5 points
- **Different but same area**: 3 points
- **No postal code data**: 2 points (neutral)
- **Different area**: 0 points

#### 9. Vegan/Vegetarian Relevance (5 points)
- **Clear vegan/vegetarian indicators**: 5 points
- **Likely appropriate**: 3 points
- **Unclear relevance**: 1 point
- **Not appropriate for HappyCow**: 0 points

## Auto-Decision Logic

### Decision Tree
1. **Check User Authority**
   ```
   IF venue_owner AND has_critical_data THEN auto_approve
   ELSE IF high_ambassador AND has_critical_data THEN auto_approve
   ELSE apply_scoring_system
   ```

2. **Scoring Thresholds** (after authority bonuses)
    - **85+ points**: ✅ **Auto-approve** (set `active = 1`)
    - **50-84 points**: ⚠️ **Manual review required** (keep `active = 0`)
    - **Below 50 points**: ❌ **Auto-reject** (set `active = -1`)

### Data Enhancement Rules
- **Always fill missing data** from Google Places when available
- **For authority users**: Trust their data over Google when conflicts exist
- **For regular users**: Trust Google data over user data when conflicts exist
- **Log all data sources** and conflicts for audit trail

## Special Cases for Manual Review
- **Korean/Chinese venues** (language barriers) - Always manual review unless venue owner
- **Venues with minimal Google data** - Manual review
- **New businesses** (less than 6 months old) - Manual review
- **Venues with conflicting information** - Manual review (unless authority user)
- **Vegan/vegetarian relevance unclear** - Manual review

## Implementation Requirements

### Core Components
1. **User Authority Checker**
    - Check venue_admin relationship
    - Determine ambassador status and geographic region
    - Calculate authority level and bonuses

2. **Venue Processor**
    - Fetch pending venues with user information
    - Apply authority-based validation logic
    - Concurrent processing with rate limiting

3. **Google Maps Scraper**
    - Places API integration for venue lookup
    - Extract all validation fields
    - Handle API rate limits and errors

4. **AI Scorer**
    - Authority-aware scoring system
    - Implement detailed scoring criteria with bonuses
    - Generate validation reports with reasoning

5. **Data Merger**
    - Merge Google data with user-submitted data
    - Apply trust rules based on user authority
    - Fill missing fields intelligently

6. **Admin Interface**
    - Display validation results with user authority info
    - Show data source conflicts and resolutions
    - Manual override capabilities
    - Processing status and statistics

### Database Queries Needed
- Get venue with submitter info: `venues JOIN members ON venues.user_id = members.id`
- Check venue ownership: `venue_admin WHERE venue_id = ? AND user_id = ?`
- Get ambassador status: `ambassadors WHERE user_id = ? AND path LIKE '%country%'`
- Get user trust level: `members.trusted, members.contributions`

### Performance Specifications
- **Concurrency**: Process 10+ venues simultaneously
- **Rate Limiting**: Respect Google Places API limits
- **Batch Processing**: Handle 7000+ venues efficiently
- **Error Recovery**: Graceful handling of API failures
- **Logging**: Detailed processing logs with authority decisions

## Success Criteria
- ✅ **Efficiency**: Process 7000+ venues in hours vs weeks
- ✅ **Accuracy**: Maintain quality standards while reducing manual work
- ✅ **Authority Recognition**: Properly identify and trust venue owners/ambassadors
- ✅ **Cost**: Stay under $200 in API costs
- ✅ **Deployment**: Single binary deployment behind existing infrastructure
- ✅ **Reduction**: Under 1000 venues requiring manual review (80%+ reduction)

## Risk Mitigation
- **No browser automation** - Use direct API calls for simplicity
- **Conservative authority checking** - Verify user credentials carefully
- **Fallback to manual review** for edge cases
- **Korean/Chinese venues** considered edge cases unless venue owner
- **Audit trail** for all authority-based decisions
- **No blocking dependencies** on other urgent tasks

## Resource Requirements
- **Development**: 2 days
- **Collaboration**: Devon / Erika / Yoke Mun for testing
- **Infrastructure**: Existing VPN & Nginx setup
- **API Costs**: < $200 (Google Places + OpenAI)
- **Deployment**: Single Go binary