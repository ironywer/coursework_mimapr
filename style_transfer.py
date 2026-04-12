import torch
import torch.nn as nn
import torch.optim as optim
import torchvision.transforms as transforms
from torchvision.models import vgg19, VGG19_Weights
from PIL import Image
import sys
import os

device = torch.device("cuda" if torch.cuda.is_available() else "cpu")

# -----------------------------
# Загрузка и преобразование изображения
# -----------------------------
def load_image(path, max_size=512):
    image = Image.open(path).convert("RGB")

    transform = transforms.Compose([
        transforms.Resize(max_size),
        transforms.CenterCrop(max_size),
        transforms.ToTensor(),
    ])

    image = transform(image).unsqueeze(0)
    return image.to(device)

def save_output(tensor, path):
    image = tensor.detach().cpu().clone().squeeze(0)
    image = image.clamp(0, 1)
    image = transforms.ToPILImage()(image)
    image.save(path)

# -----------------------------
# Нормализация под VGG
# -----------------------------
class Normalization(nn.Module):
    def __init__(self):
        super().__init__()
        mean = torch.tensor([0.485, 0.456, 0.406]).view(1, 3, 1, 1)
        std = torch.tensor([0.229, 0.224, 0.225]).view(1, 3, 1, 1)
        self.register_buffer("mean", mean)
        self.register_buffer("std", std)

    def forward(self, x):
        return (x - self.mean) / self.std

# -----------------------------
# VGG для извлечения признаков
# -----------------------------
class VGGFeatures(nn.Module):
    def __init__(self):
        super().__init__()
        self.normalization = Normalization()
        self.model = vgg19(weights=VGG19_Weights.DEFAULT).features.eval()

        for param in self.model.parameters():
            param.requires_grad_(False)

        # style layers: conv1_1, conv2_1, conv3_1, conv4_1, conv5_1
        self.style_layers = {'0', '5', '10', '19', '28'}
        # content layer: conv4_2
        self.content_layer = '21'

    def forward(self, x):
        x = self.normalization(x)

        style_features = {}
        content_feature = None

        for layer_num, layer in enumerate(self.model):
            x = layer(x)
            layer_num = str(layer_num)

            if layer_num in self.style_layers:
                style_features[layer_num] = x
            if layer_num == self.content_layer:
                content_feature = x

        return style_features, content_feature

# -----------------------------
# Loss functions
# -----------------------------
def gram_matrix(x):
    b, c, h, w = x.shape
    features = x.view(c, h * w)
    G = torch.mm(features, features.t())
    return G / (c * h * w)

def content_loss(generated, target):
    return torch.mean((generated - target) ** 2)

def style_loss(generated, style):
    G = gram_matrix(generated)
    A = gram_matrix(style)
    return torch.mean((G - A) ** 2)

def total_variation_loss(x):
    loss_h = torch.mean(torch.abs(x[:, :, 1:, :] - x[:, :, :-1, :]))
    loss_w = torch.mean(torch.abs(x[:, :, :, 1:] - x[:, :, :, :-1]))
    return loss_h + loss_w

# -----------------------------
# Извлечение признаков стиля
# -----------------------------
def extract_style(style_image_path, out_tensor_path):
    try:
        model = VGGFeatures().to(device).eval()
        image = load_image(style_image_path)

        with torch.no_grad():
            style_features, _ = model(image)
            style_features = {k: v.detach().cpu() for k, v in style_features.items()}

        torch.save(style_features, out_tensor_path)
        print(f"✅ Признаки стиля сохранены в {out_tensor_path}")

    except Exception as e:
        print(f"❌ Ошибка извлечения стиля: {e}", file=sys.stderr)
        if os.path.exists(out_tensor_path):
            try:
                os.remove(out_tensor_path)
            except Exception:
                pass
        sys.exit(1)

# -----------------------------
# Применение стиля
# -----------------------------
def apply_style(content_path, style_tensor_path, output_path):
    print(device)

    model = VGGFeatures().to(device).eval()
    content = load_image(content_path)

    style_features = torch.load(style_tensor_path, weights_only=False)
    style_features = {k: v.to(device) for k, v in style_features.items()}

    with torch.no_grad():
        _, content_target = model(content)

    generated = content.clone().requires_grad_(True)

    optimizer = optim.Adam([generated], lr=0.01)

    epochs = int(os.environ.get("EPOCHS", 1000))
    if epochs <= 0:
        print("❌ Неверное значение EPOCHS. Должно быть больше 0.", file=sys.stderr)
        sys.exit(1)

    style_weights = {
        '0': 0.15,
        '5': 0.2,
        '10': 0.25,
        '19': 0.25,
        '28': 0.15
    }

    content_weight = 1.0
    style_weight = 1e7
    tv_weight = 1e-5

    try:
        for i in range(epochs):
            gen_style_features, gen_content_feature = model(generated)

            c_loss = content_loss(gen_content_feature, content_target)

            s_loss = 0.0
            for layer in style_weights:
                s_loss += style_weights[layer] * style_loss(
                    gen_style_features[layer],
                    style_features[layer]
                )

            tv_loss = total_variation_loss(generated)

            loss = content_weight * c_loss + style_weight * s_loss + tv_weight * tv_loss

            optimizer.zero_grad()
            loss.backward()
            optimizer.step()

            with torch.no_grad():
                generated.clamp_(0, 1)

            if i % 100 == 0:
                print(
                    f"[{i}/{epochs}] "
                    f"total={loss.item():.4f} "
                    f"content={c_loss.item():.4f} "
                    f"style={s_loss.item():.6f} "
                    f"tv={tv_loss.item():.6f}"
                )

        save_output(generated, output_path)
        print(f"✅ Стилизация завершена. Сохранено в {output_path}")

    except Exception as e:
        print(f"❌ Ошибка во время стилизации: {e}", file=sys.stderr)
        if os.path.exists(output_path):
            try:
                os.remove(output_path)
            except OSError:
                pass
        sys.exit(1)

# -----------------------------
# CLI
# -----------------------------
if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Использование:\n"
              "  extract-style <style.jpg> <style.pt>\n"
              "  stylize <content.jpg> <style.pt> <output.jpg>")
        sys.exit(1)

    command = sys.argv[1]

    if command == "extract-style" and len(sys.argv) == 4:
        extract_style(sys.argv[2], sys.argv[3])

    elif command == "stylize" and len(sys.argv) == 5:
        apply_style(sys.argv[2], sys.argv[3], sys.argv[4])

    else:
        print("❌ Неверные аргументы.")
        sys.exit(1)