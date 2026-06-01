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
├── cmd/                      # Command line applications
│   ├── server/              # Main API server
│   └── migrate/             # Database migrations
├── internal/                # Private application code
│   ├── handlers/            # HTTP request handlers
│   ├── services/            # Business logic
│   ├── models/              # Data models
│   ├── repository/          # Data access layer
│   └── middleware/          # HTTP middleware
├── pkg/                     # Public packages
├── migrations/              # Database migration files
├── config/                  # Configuration management
├── tests/                   # Test files
└── docs/                    # API documentation
```

## 🔌 API Endpoints

### Authentication
- `POST /api/v1/auth/register` - Register new user
- `POST /api/v1/auth/login` - User login
- `POST /api/v1/auth/logout` - User logout
- `POST /api/v1/auth/refresh` - Refresh token

### Products
- `GET /api/v1/products` - List products with filtering and pagination
- `GET /api/v1/products/:id` - Get product details
- `POST /api/v1/products` - Create product (admin only)
- `PUT /api/v1/products/:id` - Update product (admin only)
- `DELETE /api/v1/products/:id` - Delete product (admin only)

### Cart
- `GET /api/v1/cart` - Get user's cart
- `POST /api/v1/cart/items` - Add item to cart
- `PUT /api/v1/cart/items/:id` - Update cart item
- `DELETE /api/v1/cart/items/:id` - Remove item from cart

### Orders
- `POST /api/v1/orders` - Create order
- `GET /api/v1/orders` - List user's orders
- `GET /api/v1/orders/:id` - Get order details
- `PUT /api/v1/orders/:id/status` - Update order status (admin only)

### Users
- `GET /api/v1/users/profile` - Get user profile
- `PUT /api/v1/users/profile` - Update user profile
- `GET /api/v1/users/addresses` - Get user addresses
- `POST /api/v1/users/addresses` - Add address

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
```

## 🧪 Testing

Run the test suite:

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

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

## 🔐 Security

- JWT-based authentication
- Password hashing with bcrypt
- CORS protection
- Rate limiting
- SQL injection prevention through parameterized queries
- Input validation and sanitization

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
- [ ] Real-time notifications

## 👨‍💻 Author

**Schick Backend** - A modern e-commerce platform

---

**Built with ❤️ using Go**
