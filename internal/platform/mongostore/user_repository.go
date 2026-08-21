// Package mongostore implements the app.UserRepository port on top of
// MongoDB. Everything MongoDB-specific lives here: BSON tags, ObjectID
// handling, the unique email index and driver error translation. Nothing
// above this package needs to know that MongoDB exists.
package mongostore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/domain"
)

// Compile-time proof that the adapter satisfies the port.
var _ app.UserRepository = (*UserRepository)(nil)

// userDoc is the BSON projection of domain.User. Keeping it separate stops
// storage concerns (ObjectID, bson tags) from leaking into the domain.
type userDoc struct {
	ID        bson.ObjectID `bson:"_id,omitempty"`
	Name      string        `bson:"name"`
	Email     string        `bson:"email"`
	Password  string        `bson:"password"`
	CreatedAt time.Time     `bson:"created_at"`
}

func (d userDoc) toDomain() domain.User {
	return domain.User{
		ID:        d.ID.Hex(),
		Name:      d.Name,
		Email:     d.Email,
		Password:  d.Password,
		CreatedAt: d.CreatedAt,
	}
}

// UserRepository is the MongoDB implementation of app.UserRepository.
type UserRepository struct {
	coll *mongo.Collection
}

// NewUserRepository binds the repository to the "users" collection of db
// and makes sure the unique email index exists. Creating an index is
// idempotent, so running it on every startup is safe.
func NewUserRepository(ctx context.Context, db *mongo.Database) (*UserRepository, error) {
	coll := db.Collection("users")
	_, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_email"),
	})
	if err != nil {
		return nil, fmt.Errorf("creating unique email index: %w", err)
	}
	return &UserRepository{coll: coll}, nil
}

// nowUTC returns the current time exactly as MongoDB will store it: UTC
// with millisecond precision. Using it up front means a returned user
// always round-trips through the database unchanged.
func nowUTC() time.Time {
	return time.Now().UTC().Truncate(time.Millisecond)
}

func (r *UserRepository) Create(ctx context.Context, u domain.User) (domain.User, error) {
	id := bson.NewObjectID()
	now := nowUTC()
	_, err := r.coll.InsertOne(ctx, userDoc{
		ID:        id,
		Name:      u.Name,
		Email:     u.Email,
		Password:  u.Password,
		CreatedAt: now,
	})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return domain.User{}, fmt.Errorf("%w: %s", domain.ErrEmailAlreadyExists, u.Email)
		}
		return domain.User{}, fmt.Errorf("inserting user: %w", err)
	}
	created := u
	created.ID = id.Hex()
	created.CreatedAt = now
	return created, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (domain.User, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.User{}, domain.ErrInvalidUserID
	}
	return r.findOne(ctx, bson.M{"_id": oid})
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	return r.findOne(ctx, bson.M{"email": email})
}

func (r *UserRepository) findOne(ctx context.Context, filter bson.M) (domain.User, error) {
	var doc userDoc
	if err := r.coll.FindOne(ctx, filter).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return domain.User{}, domain.ErrUserNotFound
		}
		return domain.User{}, fmt.Errorf("finding user: %w", err)
	}
	return doc.toDomain(), nil
}

func (r *UserRepository) List(ctx context.Context, limit, offset int64) ([]domain.User, int64, error) {
	total, err := r.coll.CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, 0, fmt.Errorf("counting users: %w", err)
	}
	if total == 0 || offset >= total {
		return []domain.User{}, total, nil
	}

	cursor, err := r.coll.Find(ctx, bson.M{},
		options.Find().
			SetSort(bson.D{{Key: "created_at", Value: 1}, {Key: "_id", Value: 1}}).
			SetLimit(limit).
			SetSkip(offset))
	if err != nil {
		return nil, 0, fmt.Errorf("listing users: %w", err)
	}
	var docs []userDoc
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, 0, fmt.Errorf("decoding users: %w", err)
	}

	users := make([]domain.User, len(docs))
	for i, d := range docs {
		users[i] = d.toDomain()
	}
	return users, total, nil
}

func (r *UserRepository) Update(ctx context.Context, u domain.User) (domain.User, error) {
	oid, err := bson.ObjectIDFromHex(u.ID)
	if err != nil {
		return domain.User{}, domain.ErrInvalidUserID
	}
	// ReplaceOne keeps this simple: the caller passes the full user read
	// earlier, so password and created_at come back unchanged.
	res, err := r.coll.ReplaceOne(ctx, bson.M{"_id": oid}, userDoc{
		ID:        oid,
		Name:      u.Name,
		Email:     u.Email,
		Password:  u.Password,
		CreatedAt: u.CreatedAt,
	})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return domain.User{}, fmt.Errorf("%w: %s", domain.ErrEmailAlreadyExists, u.Email)
		}
		return domain.User{}, fmt.Errorf("replacing user: %w", err)
	}
	if res.MatchedCount == 0 {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, nil
}

func (r *UserRepository) Delete(ctx context.Context, id string) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.ErrInvalidUserID
	}
	res, err := r.coll.DeleteOne(ctx, bson.M{"_id": oid})
	if err != nil {
		return fmt.Errorf("deleting user: %w", err)
	}
	if res.DeletedCount == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}

func (r *UserRepository) Count(ctx context.Context) (int64, error) {
	n, err := r.coll.CountDocuments(ctx, bson.M{})
	if err != nil {
		return 0, fmt.Errorf("counting users: %w", err)
	}
	return n, nil
}
