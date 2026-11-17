package maps

import (
	notificationpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/notification"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
)

func MapProtoNotificationToGraphQL(n *notificationpb.Notification) *model.Notification {
	if n == nil {
		return nil
	}

	return &model.Notification{
		ID:            n.GetId(),
		UserID:        n.GetUserId(),
		Channel:       n.GetChannel(),
		EventType:     optStr(n.GetEventType()),
		EventID:       optStr(n.GetEventId()),
		Recipient:     optStr(n.GetRecipient()),
		Subject:       n.GetSubject(),
		Content:       n.GetContent(),
		Status:        n.GetStatus(),
		ErrorMessage:  optStr(n.GetErrorMessage()),
		IsRead:        n.GetIsRead(),
		ReadAt:        optStr(n.GetReadAt()),
		SentAt:        optStr(n.GetSentAt()),
		CorrelationID: optStr(n.GetCorrelationId()),
		CreatedAt:     optStr(n.GetCreatedAt()),
		UpdatedAt:     optStr(n.GetUpdatedAt()),
	}
}

func MapProtoPushSubscriptionToGraphQL(sub *notificationpb.PushSubscription) *model.PushSubscription {
	if sub == nil {
		return nil
	}

	return &model.PushSubscription{
		ID:            sub.GetId(),
		UserID:        sub.GetUserId(),
		Endpoint:      sub.GetEndpoint(),
		UserAgent:     optStr(sub.GetUserAgent()),
		DeviceName:    optStr(sub.GetDeviceName()),
		IsActive:      sub.GetIsActive(),
		FailureCount:  int(sub.GetFailureCount()),
		LastError:     optStr(sub.GetLastError()),
		LastUsedAt:    optStr(sub.GetLastUsedAt()),
		DeactivatedAt: optStr(sub.GetDeactivatedAt()),
		CreatedAt:     optStr(sub.GetCreatedAt()),
		UpdatedAt:     optStr(sub.GetUpdatedAt()),
	}
}

func optStr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
