package middlewares

import (
	"context"
	"fmt"
	"math"
	"net"
	"time"

	"github.com/oschwald/geoip2-golang"
	"golang.org/x/exp/rand"

	"github.com/KhoshMaze/khoshmaze-backend/api/utils"
	"github.com/KhoshMaze/khoshmaze-backend/internal/adapters/cache"
	appContext "github.com/KhoshMaze/khoshmaze-backend/internal/adapters/context"
	"github.com/KhoshMaze/khoshmaze-backend/internal/domain/user/model"
	"github.com/KhoshMaze/khoshmaze-backend/internal/domain/user/port"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

const earthRadius = 6371.0

type geoLocation struct {
	IP        string
	Latitude  float64
	Longitude float64
	Timestamp time.Time
}

type GeoAnomalyService struct {
	rdb         cache.Provider
	ttl         time.Duration // minutes
	maxSpeed    float64       // KM/H
	maxDistance float64       // KM
	dbPath      string
	userSvc     port.Service
}

func NewGeoAnomalyService(rdb cache.Provider, ttl time.Duration, maxSpeed float64, maxDistance float64, dbPath string, userSvc port.Service) *GeoAnomalyService {
	return &GeoAnomalyService{
		rdb:         rdb,
		ttl:         ttl,
		maxSpeed:    maxSpeed,
		maxDistance: maxDistance,
		dbPath:      dbPath,
		userSvc:     userSvc,
	}
}

func deg2rad(deg float64) float64 {
	return deg * math.Pi / 180
}

func (ga *GeoAnomalyService) calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {

	dLat := deg2rad(lat2 - lat1)
	dLon := deg2rad(lon2 - lon1)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(deg2rad(lat1))*math.Cos(deg2rad(lat2))*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadius * c
}

func (ga *GeoAnomalyService) isSpeedSuspicious(loc1, loc2 geoLocation) bool {
	distance := ga.calculateDistance(loc1.Latitude, loc1.Longitude, loc2.Latitude, loc2.Longitude)

	if distance > 200 {
		return true
	}

	if distance < ga.maxDistance {
		return false
	}

	timeDiff := loc2.Timestamp.Sub(loc1.Timestamp).Hours()

	if timeDiff <= 1 {
		return distance > ga.maxDistance
	}
	speed := distance / timeDiff

	return speed > ga.maxSpeed
}

func (ga *GeoAnomalyService) storeLocation(ctx context.Context, userID uint, loc geoLocation) error {
	oc := cache.NewObjectCacher[[]geoLocation](ga.rdb, cache.SerializationTypeJSON)

	history, err := oc.Get(ctx, fmt.Sprintf("geo:history:%d", userID))

	if err != nil && err != redis.Nil {
		return err
	}
	if len(history) > 0 {
		loc2 := history[len(history)-1]
		if loc.IP == loc2.IP {
			return nil
		}
	}

	history = append(history, loc)
	err = oc.Set(ctx, fmt.Sprintf("geo:history:%d", userID), ga.ttl, history)

	if err != nil {
		logger := appContext.GetLogger(ctx).With("user_id", userID)
		appContext.SetLogger(ctx, logger)
		logger.Error("anomaly detected but failed to store location", "error", err)
		return err
	}

	return nil
}

func (ga *GeoAnomalyService) detectAnomaly(ctx context.Context, userID uint, currentLoc geoLocation) (bool, error) {

	oc := cache.NewObjectCacher[[]geoLocation](ga.rdb, cache.SerializationTypeJSON)
	history, err := oc.Get(ctx, fmt.Sprintf("geo:history:%d", userID))

	if err != nil && err != redis.Nil {
		return false, err
	}

	if len(history) == 0 {
		return false, nil
	}

	latest := history[len(history)-1]

	return ga.isSpeedSuspicious(latest, currentLoc), nil

}

func (ga *GeoAnomalyService) DetectAnomalyMiddleware(jwtSecret []byte) fiber.Handler {

	return func(c *fiber.Ctx) error {
		token := c.Cookies("refreshToken")
		userClaims, err := utils.UserClaimsFromCookies(token, jwtSecret)

		if err != nil {
			return c.SendStatus(fiber.StatusUnauthorized)
		}

		if userClaims == nil {
			return c.SendStatus(fiber.StatusUnauthorized)
		}

		ip := c.IP()
		ctx := c.UserContext()
		location, err := ga.getLocationInfo(ip)

		logger := appContext.GetLogger(ctx).With("ip", ip)
		appContext.SetLogger(ctx, logger)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, err.Error())
		}

		isSuspicious, err := ga.detectAnomaly(ctx, userClaims.UserID, *location)

		if isSuspicious {

			oc := cache.NewObjectCacher[int](ga.rdb, cache.SerializationTypeGob)

			count, err := oc.Get(ctx, fmt.Sprintf("geo:history:%d:flags", userClaims.UserID))

			if err != nil && err != redis.Nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}

			oc.Set(ctx, fmt.Sprintf("geo:history:%d:flags", userClaims.UserID), ga.ttl, count+1)
			
			min_rand_flag := 3
			max_rand_flag := 5
			// This is for the detector behavior to be less deterministic
			if count > rand.Intn(max_rand_flag-min_rand_flag+1)+min_rand_flag {

				ga.userSvc.DeleteToken(ctx, model.TokenWhitelist{
					ExpiresAt: userClaims.ExpiresAt.Time,
					Value:     token,
					UserID:    model.UserID(userClaims.UserID),
				})
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"error": "Suspicious activity detected",
				})
			}

			if err := ga.storeLocation(ctx, userClaims.UserID, *location); err != nil {
				return c.Next()
			}

		}

		return c.Next()
	}
}

func (ga *GeoAnomalyService) getLocationInfo(ip string) (*geoLocation, error) {
	db, err := geoip2.Open(ga.dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	record, err := db.City(net.ParseIP(ip))

	if err != nil {
		return nil, err
	}

	return &geoLocation{
		IP:        ip,
		Latitude:  record.Location.Latitude,
		Longitude: record.Location.Longitude,
		Timestamp: time.Now(),
	}, nil

}
