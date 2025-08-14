#ifndef OMS_TYPES_H
#define OMS_TYPES_H

#include <cstdint>
#include <string>
#include <chrono>

namespace oms {

using OrderId = uint64_t;
using ClientOrderId = std::string;
using Symbol = std::string;
using Exchange = std::string;
using Price = double;
using Quantity = double;
using Timestamp = std::chrono::microseconds;

enum class Side : uint8_t {
    BUY = 0,
    SELL = 1
};

enum class OrderType : uint8_t {
    MARKET = 0,
    LIMIT = 1,
    STOP = 2,
    STOP_LIMIT = 3,
    TAKE_PROFIT = 4,
    TAKE_PROFIT_LIMIT = 5
};

enum class OrderStatus : uint8_t {
    NEW = 0,
    PARTIALLY_FILLED = 1,
    FILLED = 2,
    CANCELED = 3,
    REJECTED = 4,
    EXPIRED = 5
};

enum class TimeInForce : uint8_t {
    GTC = 0,  // Good Till Cancel
    IOC = 1,  // Immediate or Cancel
    FOK = 2,  // Fill or Kill
    GTX = 3   // Good Till Crossing
};

enum class ExchangeType : uint8_t {
    BINANCE_SPOT = 0,
    BINANCE_FUTURES = 1,
    BYBIT_SPOT = 2,
    BYBIT_FUTURES = 3,
    OKX_SPOT = 4,
    OKX_FUTURES = 5,
    UPBIT = 6
};

struct Order {
    OrderId id;
    ClientOrderId client_order_id;
    ExchangeType exchange;
    Symbol symbol;
    Side side;
    OrderType type;
    Price price;
    Quantity quantity;
    Quantity executed_quantity;
    OrderStatus status;
    TimeInForce time_in_force;
    Timestamp created_at;
    Timestamp updated_at;
};

struct Position {
    ExchangeType exchange;
    Symbol symbol;
    Side side;
    Quantity quantity;
    Price entry_price;
    Price mark_price;
    double unrealized_pnl;
    double realized_pnl;
    double margin;
    double leverage;
    Timestamp updated_at;
};

struct MarketData {
    ExchangeType exchange;
    Symbol symbol;
    Price bid_price;
    Price ask_price;
    Quantity bid_quantity;
    Quantity ask_quantity;
    Price last_price;
    Quantity volume_24h;
    Timestamp timestamp;
};

struct RiskLimits {
    double max_position_size_usd;
    double max_leverage;
    double max_daily_loss_usd;
    double price_deviation_threshold;
    uint32_t max_orders_per_second;
    uint32_t max_orders_per_minute;
};

} // namespace oms

#endif // OMS_TYPES_H