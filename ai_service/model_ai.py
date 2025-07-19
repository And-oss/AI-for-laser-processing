import torch
import numpy as np
import cv2
import matplotlib.pyplot as plt
import torch.nn as nn
import torchvision.models as models
from PIL import Image
import os

class LaserVisionNet(nn.Module):
    def __init__(self, backbone='resnet34', num_classes=3):
        super().__init__()
        resnet = models.resnet34(weights=None)
        self.encoder = nn.Sequential(*list(resnet.children())[:-2])

        self.seg_decoder = nn.Sequential(
            nn.ConvTranspose2d(512, 256, kernel_size=2, stride=2),
            nn.ReLU(inplace=True),
            nn.ConvTranspose2d(256, 128, kernel_size=2, stride=2),
            nn.ReLU(inplace=True),
            nn.ConvTranspose2d(128, 64, kernel_size=2, stride=2),
            nn.ReLU(inplace=True),
            nn.ConvTranspose2d(64, 32, kernel_size=2, stride=2),
            nn.ReLU(inplace=True),
            nn.ConvTranspose2d(32, num_classes, kernel_size=2, stride=2)
        )

        self.regressor = nn.Sequential(
            nn.AdaptiveAvgPool2d((1, 1)),
            nn.Flatten(),
            nn.Linear(512, 128),
            nn.ReLU(inplace=True),
            nn.Linear(128, 3)
        )

    def forward(self, x):
        feats = self.encoder(x)
        seg_out = self.seg_decoder(feats)
        reg_out = self.regressor(feats)
        return seg_out, reg_out


def load_model(weights_path):
    model = LaserVisionNet()
    model.load_state_dict(torch.load(weights_path, map_location='cpu'))
    model.eval()
    return model


 
def preprocess_png(png_path):
    image = cv2.imread(png_path, cv2.IMREAD_GRAYSCALE)
    if image is None:
        raise ValueError(f"Ошибка чтения изображения: {png_path}")

    # Та же предобработка, как в обучении
    clahe = cv2.createCLAHE(clipLimit=2.0, tileGridSize=(8, 8))
    normalized = clahe.apply(image)
    blurred = cv2.GaussianBlur(normalized, (5, 5), 0)
    resized = cv2.resize(blurred, (512, 256))  # (W, H)

    # Преобразование в тензор, нормализация
    image = resized.astype(np.float32) / 255.0
    image = np.stack([image] * 3, axis=0)  # [3, H, W]
    image = torch.from_numpy(image).unsqueeze(0)  # [1, 3, H, W]

    return image


def predict_and_save_masks(model, image_tensor, output_dir, filename):
    with torch.no_grad():
        seg_out, reg_out = model(image_tensor)
        seg_logits = seg_out[0].cpu()
        pred_mask = torch.sigmoid(seg_logits).numpy()
        prediction = reg_out[0].cpu().numpy()

    os.makedirs(output_dir, exist_ok=True)
    for i in range(3):
        mask_path = os.path.join(output_dir, f"{filename}_class{i}.png")
        cv2.imwrite(mask_path, (pred_mask[i] * 255).astype(np.uint8))

    return prediction