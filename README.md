# Schick - Go Backend

A modern, full-featured e-commerce marketplace platform backend built with Go, designed for selling authentic clothing, accessories, and fashion products.

## 🎯 Overview

Schick is a comprehensive backend solution for building and managing a robust online shopping experience. Built with performance, scalability, and developer experience in mind, the Go backend provides all the essential APIs and services needed to power a production-grade e-commerce marketplace.

## ✨ Features

- **Product Management**: Comprehensive product catalog with attributes, variants, and inventory management
- **Shopping Cart & Checkout**: Seamless shopping experience with cart management and order processing
- **Payment Processing**: Integrated payment gateway support for multiple payment methods
- **User Management**: Robust authentication, authorization, and user profile management
- **Order Management**: Complete order lifecycle management and tracking
- **Search & Filtering**: Advanced product search and filtering capabilities
- **Reviews & Ratings**: Customer feedback system with reviews and ratings
- **Inventory Management**: Real-time stock tracking and management
- **Admin Dashboard APIs**: Complete admin functionality for marketplace management

## 🚀 Getting Started

### Prerequisites

- Go 1.21 or higher
- PostgreSQL or supported database
- Redis (optional, for caching)
- Docker (optional, for containerization)

### Installation

1. **Clone the repository**
   ```bash
   git clone https://github.com/elug3/schick.git
   cd schick
   ```

2. **Install dependencies**
   ```bash
   go mod download
   ```

3. **Set up environment variables**
   ```bash
   cp .env.example .env
   # Edit .env with your configuration
   ```

4. **Run database migrations**
   ```bash
   go run cmd/migrate/main.go
   ```

5. **Start the server**
   ```bash
   go run cmd/server/main.go
   ```

The API will be available at `http://localhost:8080`

## 📋 Project Structure

```
schick/
├── cmd/                           # Command line applications
│   ├── server/                   # Main API server
│   └── migrate/                  # Database migrations
├── internal/                      # Private application code
│   ├── handlers/                 # HTTP request handlers
│   ├── services/                 # Microservices layer
│   │   ├── auth-service/         # Authentication, login, token refresh, 2FA
│   │   ├── product-service/      # Product & inventory CRUD operations
│   │   ├── order-service/        # Order management & lifecycle
│   │   ├── user-service/         # Customer user management & profiles
│   │   ├── notification-service/ # Email & push notification triggers
│   │   ├── chat-service/         # Customer chat threads & messaging
│   │   ├── analytics-service/    # Reporting queries & metrics
│   │   └── config-service/       # Server settings (Super Admin only)
│   ├── models/                   # Data models & entities
│   ├── repository/               # Data access layer (DAL)
│   ├── middleware/               # HTTP middleware & interceptors
│   └── utils/                    # Utility functions & helpers
├── pkg/                           # Public packages & shared libraries
├── migrations/                    # Database migration files
├── config/                        # Configuration management
├── tests/                         # Test suites & fixtures
└── docs/                          # API documentation & Swagger specs
```

### Services Architecture

#### 🔐 Auth Service (`auth-service`)
Handles user authentication and security:
- User login & logout
- JWT token generation and refresh
- Two-factor authentication (2FA)
- Password reset & recovery
- OAuth/SSO integration points

#### 📦 Product Service (`product-service`)
Manages product catalog and inventory:
- Product CRUD operations
- Inventory management & tracking
- Product variants & attributes
- Stock level management
- Product search & categorization

#### 🛒 Order Service (`order-service`)
Manages the complete order lifecycle:
- Order creation & processing
- Order status tracking
- Order history & details
- Payment processing coordination
- Shipment & delivery tracking

#### 👤 User Service (`user-service`)
Handles customer user management:
- User profile management
- Address book management
- Wishlist & favorites
- User preferences & settings
- Customer preferences & notifications settings

#### 📬 Notification Service (`notification-service`)
Manages all notification channels:
- Email notifications (order confirmations, shipping updates)
- Push notifications (mobile alerts)
- SMS notifications (critical updates)
- Notification templates & scheduling
- Event-triggered notification system

#### 💬 Chat Service (`chat-service`)
Manages customer communications:
- Customer chat threads
- Support ticket creation & tracking
- Message history & persistence
- Real-time message delivery
- Chat thread management & resolution

#### 📊 Analytics Service (`analytics-service`)
Provides reporting and metrics:
- Sales analytics & reporting
- User behavior analytics
- Product performance metrics
- Revenue & profit analysis
- Custom report generation

#### ⚙️ Config Service (`config-service`)
Manages system configuration (Super Admin only):
- Server settings & parameters
- Feature flags & toggles
- System-wide configurations
- Audit logging for config changes
- Role-based access control

## 🔌 API Endpoints

### Authentication (`auth-service`)
- `POST /api/v1/auth/register` - Register new user
- `POST /api/v1/auth/login` - User login
- `POST /api/v1/auth/logout` - User logout
- `POST /api/v1/auth/refresh` - Refresh token
- `POST /api/v1/auth/2fa/setup` - Setup 2FA
- `POST /api/v1/auth/2fa/verify` - Verify 2FA code

### Products (`product-service`)
- `GET /api/v1/products` - List products with filtering and pagination
- `GET /api/v1/products/:id` - Get product details
- `POST /api/v1/products` - Create product (admin only)
- `PUT /api/v1/products/:id` - Update product (admin only)
- `DELETE /api/v1/products/:id` - Delete product (admin only)
- `GET /api/v1/products/:id/inventory` - Get product inventory
- `PUT /api/v1/products/:id/inventory` - Update inventory

### Orders (`order-service`)
- `POST /api/v1/orders` - Create order
- `GET /api/v1/orders` - List user's orders
- `GET /api/v1/orders/:id` - Get order details
- `PUT /api/v1/orders/:id/status` - Update order status (admin only)
- `GET /api/v1/orders/:id/tracking` - Get order tracking info

### Users (`user-service`)
- `GET /api/v1/users/profile` - Get user profile
- `PUT /api/v1/users/profile` - Update user profile
- `GET /api/v1/users/addresses` - Get user addresses
- `POST /api/v1/users/addresses` - Add address
- `GET /api/v1/users/wishlist` - Get wishlist

### Notifications (`notification-service`)
- `GET /api/v1/notifications` - Get user notifications
- `POST /api/v1/notifications/preferences` - Update notification preferences
- `PUT /api/v1/notifications/:id/read` - Mark notification as read

### Chat (`chat-service`)
- `GET /api/v1/chat/threads` - List chat threads
- `POST /api/v1/chat/threads` - Create new chat thread
- `GET /api/v1/chat/threads/:id/messages` - Get thread messages
- `POST /api/v1/chat/threads/:id/messages` - Send message

### Analytics (`analytics-service`)
- `GET /api/v1/analytics/sales` - Sales analytics (admin only)
- `GET /api/v1/analytics/products` - Product performance (admin only)
- `GET /api/v1/analytics/users` - User metrics (admin only)
- `GET /api/v1/analytics/revenue` - Revenue reports (admin only)

### Config (`config-service`)
- `GET /api/v1/config/settings` - Get system settings (Super Admin only)
- `PUT /api/v1/config/settings` - Update settings (Super Admin only)
- `GET /api/v1/config/features` - Get feature flags (Super Admin only)
- `PUT /api/v1/config/features/:name` - Toggle feature flag (Super Admin only)

## 🛠️ Configuration

Configuration is managed through environment variables. See `.env.example` for all available options:

```env
# Server
SERVER_HOST=0.0.0.0
SERVER_PORT=8080
SERVER_ENV=development

# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=schick
DB_PASSWORD=yourpassword
DB_NAME=schick_db

# JWT
JWT_SECRET=your-secret-key
JWT_EXPIRATION=24h

# Redis (optional)
REDIS_HOST=localhost
REDIS_PORT=6379

# Email (Notification Service)
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=your-email@gmail.com
SMTP_PASSWORD=your-password

# 2FA
2FA_ISSUER=Schick
TOTP_WINDOW=1
```

## 🧪 Testing

Run the test suite:

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific service tests
go test ./internal/services/auth-service -v
go test ./internal/services/product-service -v

# Run specific test
go test ./internal/handlers -v
```

## 📦 Dependencies

Key dependencies include:
- **gin** - HTTP web framework
- **gorm** - ORM for database operations
- **jwt-go** - JWT authentication
- **viper** - Configuration management
- **zap** - Structured logging
- **testify** - Testing utilities
- **redis** - Caching & session management
- **gomail** - Email notifications

## 🔐 Security

- JWT-based authentication
- Password hashing with bcrypt
- Two-factor authentication (2FA) support
- CORS protection
- Rate limiting
- SQL injection prevention through parameterized queries
- Input validation and sanitization
- Role-based access control (RBAC)
- Super Admin access restrictions for config service

## 📝 API Documentation

For detailed API documentation, visit:
- Swagger/OpenAPI docs: `http://localhost:8080/swagger/index.html`
- API reference: See `docs/` directory

## 🤝 Contributing

Contributions are welcome! Please follow these steps:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📄 License

This project is licensed under the MIT License - see the LICENSE file for details.

## 🆘 Support

For support, please:
- Open an issue on GitHub
- Check existing documentation
- Review API documentation

## 🗺️ Roadmap

- [ ] Advanced search with Elasticsearch
- [ ] Recommendation engine
- [ ] Multi-currency support
- [ ] Webhook support
- [ ] Analytics dashboard
- [ ] GraphQL API
- [ ] Real-time notifications with WebSocket
- [ ] AI-powered chat support

## 👨‍💻 Author

**Schick Backend** - A modern e-commerce platform

---

**Built with ❤️ using Go**
