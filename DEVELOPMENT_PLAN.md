# HappyCow Venue Validation - Development Plan

## Project Status Overview
**Timeline**: 2 days  
**Current Phase**: Implementation & Testing  
**Target**: 80% reduction in manual reviews (7000+ → <1000 venues)

## Phase 1: Foundation Setup ✅
- [x] Project structure with internal packages
- [x] Go modules and basic dependencies
- [x] Configuration management with environment variables
- [x] Basic database connection setup
- [x] Project documentation (PROJECT_SPEC.md)

## Phase 2: Core Implementation (In Progress)

### 2.1 Database Layer Updates
- [ ] **Update Venue model** to match complete venues table schema
    - Add all fields: `zipcode`, `openhours`, `timezone`, `additionalinfo`, `vdetails`
    - Include metadata fields: `admin_note`, `admin_last_update`, `made_active_by_id`
    - Add validation result fields for scoring breakdown
- [ ] **Enhanced database queries**
    - Optimize pending venues query with proper indexing
    - Add batch update methods for concurrent processing
    - Implement validation history tracking
- [ ] **SQL optimization**
    - Use existing indexes (name, location, active)
    - Implement prepared statements for performance

### 2.2 Google Maps Integration Enhancement
- [ ] **Complete Places API integration**
    - Fetch all required fields: name, formatted_address, formatted_phone_number
    - Extract geometry.location for distance calculations
    - Get opening_hours, website, business_status
    - Parse address_components for postal code extraction
- [ ] **Error handling & rate limiting**
    - Implement exponential backoff for API failures
    - Handle OVER_QUERY_LIMIT responses gracefully
    - Add timeout handling for slow responses
- [ ] **Data normalization**
    - Standardize phone number formats for comparison
    - Normalize address formats (street abbreviations, etc.)
    - Handle timezone conversions for opening hours

### 2.3 AI Scoring System Implementation
- [ ] **Detailed scoring logic** (9 criteria, 100 points total)
    - Venue Name Matching (25 points) - fuzzy string matching
    - Address Accuracy (20 points) - geographic distance + text similarity
    - Geolocation Accuracy (15 points) - haversine distance calculation
    - Phone Number Verification (10 points) - normalized comparison
    - Business Hours Verification (10 points) - time range comparison
    - Website Verification (5 points) - URL domain matching
    - Business Status (5 points) - operational status check
    - Postal Code (5 points) - exact/area matching
    - Vegan/Vegetarian Relevance (5 points) - AI content analysis
- [ ] **OpenAI integration optimization**
    - Use GPT-3.5-turbo for cost efficiency
    - Implement structured prompts for consistent scoring
    - Add response validation and fallback scoring
- [ ] **Score validation & reasoning**
    - Generate detailed explanations for each score
    - Implement confidence levels for edge cases
    - Add manual review flags for uncertain cases

## Phase 3: Processing Engine

### 3.1 Concurrent Processing System
- [ ] **Worker pool implementation**
    - Configurable worker count (default: 10 concurrent)
    - Job queue with priority handling (Korean/Chinese venues)
    - Graceful shutdown and cleanup
- [ ] **Rate limiting & throttling**
    - Google Places API: 1000 requests/day free tier
    - OpenAI API: Budget-aware request limiting
    - Implement request batching where possible
- [ ] **Error recovery & retry logic**
    - Retry failed API calls with exponential backoff
    - Skip and log permanently failed venues
    - Resume processing from last successful checkpoint

### 3.2 Decision Engine
- [ ] **Auto-approval logic**
    - 85+ points: Auto-approve (`active = 1`)
    - 50-84 points: Flag for manual review (`active = 0`)
    - <50 points: Auto-reject with detailed reasoning (`active = -1`)
- [ ] **Special case handling**
    - Korean/Chinese venues → Always manual review
    - New businesses (<6 months) → Manual review
    - Missing critical data → Manual review
    - Conflicting information → Manual review

## Phase 4: Admin Interface Enhancement

### 4.1 Validation Dashboard
- [ ] **Processing status overview**
    - Real-time progress indicators
    - Success/failure/pending counters
    - Processing speed metrics
- [ ] **Venue comparison interface**
    - Side-by-side HappyCow vs Google data display
    - Color-coded field matching indicators
    - Score breakdown visualization
    - Distance calculations with map display

### 4.2 Manual Review Interface
- [ ] **Detailed venue review pages**
    - Complete validation report display
    - Manual override controls (approve/reject/hold)
    - Bulk action capabilities
    - Admin notes and history
- [ ] **Filtering & search capabilities**
    - Filter by score ranges, countries, validation status
    - Search by venue name, location, or ID
    - Export capabilities for reporting

## Phase 5: Production Readiness

### 5.1 Performance Optimization
- [ ] **Memory management**
    - Implement venue processing in batches
    - Clear processed data from memory
    - Monitor memory usage during large batches
- [ ] **Database optimization**
    - Connection pooling for concurrent access
    - Batch database updates to reduce queries
    - Index optimization for search queries
- [ ] **API cost optimization**
    - Cache Google Places results to avoid duplicate calls
    - Use Places API fields parameter to limit data transfer
    - Implement smart retry logic to minimize failed requests

### 5.2 Monitoring & Logging
- [ ] **Comprehensive logging**
    - Structured logging with log levels
    - Request/response logging for debugging
    - Performance metrics and timing
- [ ] **Health checks & monitoring**
    - Database connection health checks
    - API availability monitoring
    - Processing queue status endpoints
- [ ] **Error tracking**
    - Categorize and count error types
    - Alert on critical failures
    - Generate daily processing reports

### 5.3 Deployment Preparation
- [ ] **Build system**
    - Single binary compilation with embedded assets
    - Cross-platform build scripts
    - Version tagging and release process
- [ ] **Configuration management**
    - Environment-specific configs
    - Secrets management for API keys
    - Runtime configuration validation
- [ ] **Documentation**
    - API documentation
    - Deployment instructions
    - Operational runbooks

## Testing Strategy

### Unit Testing
- [ ] Database layer tests with test database
- [ ] Scoring algorithm tests with known venues
- [ ] API integration tests with mock responses
- [ ] Validation logic tests with edge cases

### Integration Testing
- [ ] End-to-end venue processing workflow
- [ ] Admin interface functionality
- [ ] API rate limiting and error handling
- [ ] Database performance under load

### User Acceptance Testing
- [ ] Devon/Erika/Yoke Mun testing sessions
- [ ] Validation accuracy comparison with manual reviews
- [ ] Performance testing with sample venue batches
- [ ] Edge case validation (Korean/Chinese venues)

## Risk Mitigation & Contingency Plans

### Technical Risks
- **API Rate Limits**: Implement aggressive caching and request optimization
- **API Cost Overrun**: Real-time cost monitoring with automatic shutoffs
- **Database Performance**: Connection pooling and query optimization
- **Memory Issues**: Batch processing and garbage collection monitoring

### Business Risks
- **False Positives**: Conservative scoring with manual review fallbacks
- **Data Quality**: Comprehensive validation and human oversight
- **Korean/Chinese Venues**: Dedicated manual review process
- **Integration Issues**: Phased rollout with rollback capabilities

## Success Metrics & KPIs
- **Processing Speed**: Target 1000+ venues per hour
- **Accuracy Rate**: >95% agreement with manual validation
- **Cost Efficiency**: <$200 total API costs
- **Manual Review Reduction**: 80% reduction (7000+ → <1000)
- **System Uptime**: >99% availability during processing

## Deployment Checklist
- [ ] Environment variables configured
- [ ] Database migrations applied
- [ ] API keys validated and working
- [ ] VPN and Nginx configuration updated
- [ ] Monitoring and alerting configured
- [ ] Backup and rollback procedures tested
- [ ] Team training completed
- [ ] Go-live approval from stakeholders

## Post-Launch Tasks
- [ ] Monitor initial batch processing results
- [ ] Collect feedback from manual reviewers
- [ ] Fine-tune scoring thresholds based on results
- [ ] Optimize performance based on real-world usage
- [ ] Plan for scaling to handle larger venue volumes